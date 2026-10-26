package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JaniL/tenttiarkisto/internal/config"
	"github.com/JaniL/tenttiarkisto/internal/db"
	"github.com/JaniL/tenttiarkisto/internal/geogate"
	"github.com/JaniL/tenttiarkisto/internal/models"
	"github.com/JaniL/tenttiarkisto/internal/storage"
	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

var pageNames = []string{
	"subjects_index", "subject_show", "subject_form",
	"courses_index", "course_show", "course_form",
	"exams_index", "exam_show", "exam_form",
	"approvelisting",
	"login", "register", "account",
	"users_index", "user_show", "user_form",
	"admin", "error",
}

// PageData carries template payloads. Common fields are filled by
// render(); page handlers fill the specific ones.
type PageData struct {
	Title        string
	Header       string
	Action       string
	Query        string
	Page         int
	PrevPage     int
	NextPage     int
	TotalPages   int
	PageNumbers  []int
	IsFrontpage  bool
	User         *models.User
	IsSuperAdmin bool
	Flash        string
	FlashKind    string
	CSRFField    template.HTML
	CanManage    bool
	Errors       []string

	Subjects     []models.Subject
	Subject      *models.Subject
	Courses      []models.Course
	Course       *models.Course
	Exams        []models.Exam
	Exam         *models.Exam
	FileName     string
	Users        []models.User
	UserSubjects []models.Subject
	SelSubjects  map[int64]bool
	SelSubject   int64
}

type Web struct {
	cfg      config.Config
	store    *models.Store
	sessions *scs.SessionManager
	gate     *geogate.Gate
	s3       *storage.Client
	logger   *slog.Logger
	pages    map[string]*template.Template
}

func New(cfg config.Config, store *models.Store, gate *geogate.Gate, s3 *storage.Client, logger *slog.Logger) (*Web, error) {
	sm := scs.New()
	sm.Store = pgxstore.New(store.Pool())
	sm.Cookie.Name = "tenttiarkisto_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = cfg.CookieSecure
	sm.Lifetime = 24 * time.Hour

	w := &Web{
		cfg:      cfg,
		store:    store,
		sessions: sm,
		gate:     gate,
		s3:       s3,
		logger:   logger,
	}
	pages, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	w.pages = pages
	return w, nil
}

func parseTemplates() (map[string]*template.Template, error) {
	base := template.New("layout.html")
	files := []string{"templates/layout.html", "templates/partials.html"}
	for _, f := range files {
		if _, err := base.ParseFS(templatesFS, f); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
	}
	pages := map[string]*template.Template{}
	for _, name := range pageNames {
		t, err := base.ParseFS(templatesFS, "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		pages[name] = t
	}
	return pages, nil
}

// Routes assembles the middleware chain and all routes.
func (w *Web) Routes() http.Handler {
	r := chi.NewRouter()

	var handler http.Handler = r

	// CSRF protects all state-changing routes.
	handler = csrf.Protect(
		w.cfg.CSRFKey(),
		csrf.Secure(w.cfg.CookieSecure),
		csrf.CookieName("tenttiarkisto_csrf"),
		csrf.SameSite(csrf.SameSiteLaxMode),
	)(handler)

	// Session load/save next (inside CSRF so session ops get protected context too).
	handler = w.sessions.LoadAndSave(handler)

	// Geo gate outermost: reject abusive traffic before any work.
	handler = w.gate.Middleware(handler)

	// --- routes ---

	r.Get("/", w.frontpage)
	r.Get("/healthz", w.healthz)

	staticContent, _ := fs.Sub(staticFS, "static")
	staticServer := http.StripPrefix("/static/", http.FileServer(http.FS(staticContent)))
	r.Get("/static/*", func(wr http.ResponseWriter, r *http.Request) {
		staticServer.ServeHTTP(wr, r)
	})

	// auth
	r.Get("/login", w.loginForm)
	r.Post("/login", w.loginSubmit)
	r.Get("/register", w.registerForm)
	r.Post("/register", w.registerSubmit)
	r.Post("/logout", w.logout)
	r.Get("/account", w.accountForm)
	r.Post("/account", w.accountSubmit)

	// frontpage nav header link
	r.Get("/admin", w.requireSuperAdmin(w.adminPanel))

	// subjects (super admin managed)
	r.Get("/subjects", w.subjectsIndex)
	r.Get("/subjects/new", w.requireSuperAdmin(w.subjectNew))
	r.Post("/subjects", w.requireSuperAdmin(w.subjectCreate))
	r.Get("/subjects/{id}", w.subjectShow)
	r.Get("/subjects/{id}/edit", w.requireSuperAdmin(w.subjectEdit))
	r.Post("/subjects/{id}/update", w.requireSuperAdmin(w.subjectUpdate))
	r.Post("/subjects/{id}/delete", w.requireSuperAdmin(w.subjectDelete))

	// courses
	r.Get("/courses", w.coursesIndex)
	r.Get("/courses/new", w.requireLogin(w.courseNew))
	r.Post("/courses", w.requireLogin(w.courseCreate))
	r.Get("/courses/{id}", w.courseShow)
	r.Get("/courses/{id}/edit", w.requireLogin(w.courseEdit))
	r.Post("/courses/{id}/update", w.requireLogin(w.courseUpdate))
	r.Post("/courses/{id}/delete", w.requireLogin(w.courseDelete))

	// exams
	r.Get("/exams", w.examsIndex)
	r.Get("/exams/new", w.requireLogin(w.examNew))
	r.Post("/exams", w.requireLogin(w.examCreate))
	r.Get("/exams/{id}", w.examShow)
	r.Get("/exams/{id}/file", w.examFile)
	r.Get("/exams/{id}/edit", w.requireLogin(w.examEdit))
	r.Post("/exams/{id}/update", w.requireLogin(w.examUpdate))
	r.Post("/exams/{id}/delete", w.requireLogin(w.examDelete))
	r.Get("/approvelisting", w.requireLogin(w.approvelisting))
	r.Post("/exams/{id}/approve", w.requireLogin(w.examApprove))
	r.Post("/exams/{id}/reject", w.requireLogin(w.examReject))

	// users (super admin managed)
	r.Get("/users", w.requireSuperAdmin(w.usersIndex))
	r.Get("/users/new", w.requireSuperAdmin(w.userNew))
	r.Post("/users", w.requireSuperAdmin(w.userCreate))
	r.Get("/users/{id}", w.requireSuperAdmin(w.userShow))
	r.Get("/users/{id}/edit", w.requireSuperAdmin(w.userEdit))
	r.Post("/users/{id}/update", w.requireSuperAdmin(w.userUpdate))
	r.Post("/users/{id}/delete", w.requireSuperAdmin(w.userDelete))

	r.NotFound(w.notFound)

	return handler
}

// ---- common helpers ----

const pageSize = 20

func (w *Web) render(wr http.ResponseWriter, r *http.Request, status int, name string, data PageData) {
	t, ok := w.pages[name]
	if !ok {
		http.Error(wr, "template not found: "+name, http.StatusInternalServerError)
		return
	}
	user := w.currentUser(r)
	data.User = user
	data.IsSuperAdmin = user != nil && user.IsSuperAdmin
	if f := w.sessions.PopString(r.Context(), "flash"); f != "" {
		data.Flash = f
		data.FlashKind = w.sessions.PopString(r.Context(), "flash_kind")
		if data.FlashKind == "" {
			data.FlashKind = "success"
		}
	}
	if data.FlashKind == "" {
		data.FlashKind = "success"
	}
	data.CSRFField = csrf.TemplateField(r)
	data.Errors, _ = w.sessions.Pop(r.Context(), "errors").([]string)

	wr.Header().Set("Content-Type", "text/html; charset=utf-8")
	wr.WriteHeader(status)
	if err := t.ExecuteTemplate(wr, "layout.html", data); err != nil {
		w.logger.Error("render template", "template", name, "err", err)
	}
}

func (w *Web) redirect(wr http.ResponseWriter, r *http.Request, path, flash string) {
	if flash != "" {
		w.sessions.Put(r.Context(), "flash", flash)
	}
	http.Redirect(wr, r, path, http.StatusSeeOther)
}

func (w *Web) renderErrors(wr http.ResponseWriter, r *http.Request, status int, name string, data PageData, errs []string) {
	w.sessions.Put(r.Context(), "errors", errs)
	w.render(wr, r, status, name, data)
}

func (w *Web) currentUser(r *http.Request) *models.User {
	uid := w.sessions.GetInt64(r.Context(), "uid")
	if uid == 0 {
		return nil
	}
	u, err := w.store.GetUser(r.Context(), uid)
	if err != nil {
		if err != db.ErrNotFound {
			w.logger.Error("load current user", "err", err)
		}
		return nil
	}
	return &u
}

func (w *Web) requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(wr http.ResponseWriter, r *http.Request) {
		if w.currentUser(r) == nil {
			http.Redirect(wr, r, "/login", http.StatusSeeOther)
			return
		}
		next(wr, r)
	}
}

func (w *Web) requireSuperAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(wr http.ResponseWriter, r *http.Request) {
		u := w.currentUser(r)
		if u == nil {
			http.Redirect(wr, r, "/login", http.StatusSeeOther)
			return
		}
		if !u.IsSuperAdmin {
			http.Redirect(wr, r, "/", http.StatusSeeOther)
			return
		}
		next(wr, r)
	}
}

func (w *Web) fillPagination(data *PageData, page, total int) {
	if page < 1 {
		page = 1
	}
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	data.Page = page
	data.TotalPages = totalPages
	data.PrevPage = page - 1
	if data.PrevPage < 1 {
		data.PrevPage = 1
	}
	data.NextPage = page + 1
	if data.NextPage > totalPages {
		data.NextPage = totalPages
	}
	start := page - 4
	if start < 1 {
		start = 1
	}
	end := start + 8
	if end > totalPages {
		end = totalPages
	}
	data.PageNumbers = nil
	for i := start; i <= end; i++ {
		data.PageNumbers = append(data.PageNumbers, i)
	}
}

func (w *Web) pageParam(r *http.Request) int {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	return p
}

func (w *Web) queryParam(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("q"))
}

func (w *Web) pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func (w *Web) notFound(wr http.ResponseWriter, r *http.Request) {
	data := PageData{Title: "Not found", Header: "404"}
	data.Flash = "The page you were looking for does not exist."
	data.FlashKind = "danger"
	w.render(wr, r, http.StatusNotFound, "error", data)
}

func (w *Web) serverError(wr http.ResponseWriter, r *http.Request, err error) {
	w.logger.Error("server error", "err", err, "path", r.URL.Path)
	data := PageData{Title: "Error", Header: "500"}
	data.Flash = "Something went wrong on our side. Please try again later."
	data.FlashKind = "danger"
	w.render(wr, r, http.StatusInternalServerError, "error", data)
}

func (w *Web) healthz(wr http.ResponseWriter, r *http.Request) {
	if err := w.store.Ping(r.Context()); err != nil {
		wr.WriteHeader(http.StatusServiceUnavailable)
		_, _ = wr.Write([]byte("db unavailable"))
		return
	}
	_, _ = wr.Write([]byte("ok"))
}
