package web

import (
	"net/http"
	"strings"

	"github.com/JaniL/tenttiarkisto/internal/db"
	"github.com/JaniL/tenttiarkisto/internal/models"
)

// frontpage renders the subject list (home page).
func (w *Web) frontpage(wr http.ResponseWriter, r *http.Request) {
	w.subjectsIndex(wr, r) // same view; frontpage flag set inside
}

func (w *Web) subjectsIndex(wr http.ResponseWriter, r *http.Request) {
	query := w.queryParam(r)
	page := w.pageParam(r)
	subjects, total, err := w.store.ListSubjects(r.Context(), query, page)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	data := PageData{
		Title:       "Subjects",
		Header:      "Subjects",
		Action:      "/subjects",
		Query:       query,
		IsFrontpage: r.URL.Path == "/",
	}
	if data.IsFrontpage {
		data.Title = "HYY Tenttiarkisto"
		data.Header = ""
	}
	data.Subjects = subjects
	w.fillPagination(&data, page, total)
	w.render(wr, r, http.StatusOK, "subjects_index", data)
}

func (w *Web) subjectShow(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	subject, err := w.store.GetSubject(r.Context(), id)
	if err == db.ErrNotFound {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	query := w.queryParam(r)
	page := w.pageParam(r)
	courses, total, err := w.store.ListCourses(r.Context(), subject.ID, query, page)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	user := w.currentUser(r)
	data := PageData{
		Title:     subject.Name,
		Header:    subject.Name,
		Action:    "/subjects/" + itoa(subject.ID),
		Query:     query,
		Subject:   &subject,
		Courses:   courses,
		CanManage: user != nil && w.store.UserCanManageSubject(r.Context(), user, subject.ID),
	}
	w.fillPagination(&data, page, total)
	w.render(wr, r, http.StatusOK, "subject_show", data)
}

func (w *Web) subjectNew(wr http.ResponseWriter, r *http.Request) {
	w.render(wr, r, http.StatusOK, "subject_form", PageData{Title: "Add Subject", Header: "Add Subject"})
}

func (w *Web) subjectCreate(wr http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.serverError(wr, r, err)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		w.renderErrors(wr, r, http.StatusUnprocessableEntity, "subject_form",
			PageData{Title: "Add Subject", Header: "Add Subject"},
			[]string{"Name can't be blank."})
		return
	}
	sub, err := w.store.CreateSubject(r.Context(), name)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.redirect(wr, r, "/subjects/"+itoa(sub.ID), "Subject was successfully created.")
}

func (w *Web) subjectEdit(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	subject, err := w.store.GetSubject(r.Context(), id)
	if err == db.ErrNotFound {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.render(wr, r, http.StatusOK, "subject_form", PageData{
		Title: "Edit Subject", Header: subject.Name, Subject: &subject,
	})
}

func (w *Web) subjectUpdate(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.serverError(wr, r, err)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		subject, _ := w.store.GetSubject(r.Context(), id)
		w.renderErrors(wr, r, http.StatusUnprocessableEntity, "subject_form",
			PageData{Title: "Edit Subject", Header: "Edit", Subject: &subject},
			[]string{"Name can't be blank."})
		return
	}
	if err := w.store.UpdateSubject(r.Context(), id, name); err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.redirect(wr, r, "/subjects/"+itoa(id), "Subject was successfully updated.")
}

func (w *Web) subjectDelete(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	if err := w.store.DeleteSubject(r.Context(), id); err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.redirect(wr, r, "/subjects", "Subject was successfully destroyed.")
}

// ---- admin panel ----

func (w *Web) adminPanel(wr http.ResponseWriter, r *http.Request) {
	w.render(wr, r, http.StatusOK, "admin", PageData{Title: "Super Admin Panel", Header: "Admin"})
}

// ---- users (super admin) ----

func (w *Web) usersIndex(wr http.ResponseWriter, r *http.Request) {
	users, err := w.store.ListUsers(r.Context())
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.render(wr, r, http.StatusOK, "users_index", PageData{
		Title: "Users", Header: "Users", Users: users,
	})
}

func (w *Web) userNew(wr http.ResponseWriter, r *http.Request) {
	subjects, err := w.allSubjects(r)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.render(wr, r, http.StatusOK, "user_form", PageData{
		Title: "New User", Header: "New User", Subjects: subjects,
	})
}

func (w *Web) userFormSubjects(r *http.Request) ([]models.Subject, map[int64]bool, error) {
	var subjects []models.Subject
	page := 1
	for {
		more, total, err := w.store.ListSubjects(r.Context(), "", page)
		if err != nil {
			return nil, nil, err
		}
		if len(more) == 0 {
			break
		}
		subjects = append(subjects, more...)
		if page*pageSize >= total {
			break
		}
		page++
	}
	sel := map[int64]bool{}
	if err := r.ParseForm(); err == nil {
		for _, v := range r.PostForm["subject_ids"] {
			if id, err := parseInt(v); err == nil {
				sel[id] = true
			}
		}
	}
	return subjects, sel, nil
}

func (w *Web) userCreate(wr http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		w.serverError(wr, r, err)
		return
	}
	org := strings.TrimSpace(r.PostFormValue("organization"))
	email := strings.ToLower(strings.TrimSpace(r.PostFormValue("email")))
	password := r.PostFormValue("password")

	var errs []string
	if org == "" {
		errs = append(errs, "Organization can't be blank.")
	}
	if email == "" || !strings.Contains(email, "@") {
		errs = append(errs, "Email is invalid.")
	}
	if len(password) < 8 {
		errs = append(errs, "Password must be at least 8 characters.")
	}

	subjects, sel, err := w.userFormSubjects(r)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	var subjectIDs []int64
	for sid := range sel {
		subjectIDs = append(subjectIDs, sid)
	}

	if len(errs) > 0 {
		w.renderErrors(wr, r, http.StatusUnprocessableEntity, "user_form",
			PageData{Title: "New User", Header: "New User", Subjects: subjects, SelSubjects: sel}, errs)
		return
	}

	hash, err := hashPassword(password)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	u, err := w.store.CreateUser(r.Context(), org, email, hash, false, subjectIDs)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique") {
			w.renderErrors(wr, r, http.StatusUnprocessableEntity, "user_form",
				PageData{Title: "New User", Header: "New User", Subjects: subjects, SelSubjects: sel},
				[]string{"Email has already been taken."})
			return
		}
		w.serverError(wr, r, err)
		return
	}
	w.redirect(wr, r, "/users/"+itoa(u.ID), "User was successfully created.")
}

func (w *Web) userShow(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	u, err := w.store.GetUser(r.Context(), id)
	if err == db.ErrNotFound {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	var userSubjects []models.Subject
	for _, sid := range u.SubjectIDs {
		if s, err := w.store.GetSubject(r.Context(), sid); err == nil {
			userSubjects = append(userSubjects, s)
		}
	}
	w.render(wr, r, http.StatusOK, "user_show", PageData{
		Title: u.Organization, Header: u.Organization, User: &u, UserSubjects: userSubjects,
	})
}

func (w *Web) userEdit(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	u, err := w.store.GetUser(r.Context(), id)
	if err == db.ErrNotFound {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	subjects, err := w.allSubjects(r)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	sel := map[int64]bool{}
	for _, sid := range u.SubjectIDs {
		sel[sid] = true
	}
	w.render(wr, r, http.StatusOK, "user_form", PageData{
		Title: "Edit User", Header: u.Organization, User: &u,
		Subjects: subjects, SelSubjects: sel,
	})
}

func (w *Web) userUpdate(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		w.serverError(wr, r, err)
		return
	}
	u, err := w.store.GetUser(r.Context(), id)
	if err == db.ErrNotFound {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	org := strings.TrimSpace(r.PostFormValue("organization"))
	email := strings.ToLower(strings.TrimSpace(r.PostFormValue("email")))
	if email == "" {
		email = u.Email
	}

	var errs []string
	if org == "" {
		errs = append(errs, "Organization can't be blank.")
	}

	subjects, sel, err := w.userFormSubjects(r)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	var subjectIDs []int64
	for sid := range sel {
		subjectIDs = append(subjectIDs, sid)
	}

	if len(errs) > 0 {
		w.renderErrors(wr, r, http.StatusUnprocessableEntity, "user_form",
			PageData{Title: "Edit User", Header: u.Organization, User: &u,
				Subjects: subjects, SelSubjects: sel}, errs)
		return
	}

	if err := w.store.UpdateUser(r.Context(), id, org, email, nil, subjectIDs); err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.redirect(wr, r, "/users/"+itoa(id), "User was successfully updated.")
}

func (w *Web) userDelete(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	if err := w.store.DeleteUser(r.Context(), id); err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.redirect(wr, r, "/users", "User was successfully destroyed.")
}

// allSubjects loads every subject (admin tool helper).
func (w *Web) allSubjects(r *http.Request) ([]models.Subject, error) {
	var subjects []models.Subject
	page := 1
	for {
		more, total, err := w.store.ListSubjects(r.Context(), "", page)
		if err != nil {
			return nil, err
		}
		if len(more) == 0 {
			break
		}
		subjects = append(subjects, more...)
		if page*pageSize >= total {
			break
		}
		page++
	}
	return subjects, nil
}
