package web

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/JaniL/tenttiarkisto/internal/db"
	"github.com/JaniL/tenttiarkisto/internal/models"
	"golang.org/x/crypto/bcrypt"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func parseInt(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

func hashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// formDate parses a YYYY-MM-DD form value into a *time.Time.
func formDate(v string) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, fmt.Errorf("invalid date %q", v)
	}
	return &t, nil
}

func (w *Web) formInt(r *http.Request, key string) (int64, error) {
	return parseInt(strings.TrimSpace(r.PostFormValue(key)))
}

// ---- courses ----

func (w *Web) coursesIndex(wr http.ResponseWriter, r *http.Request) {
	query := w.queryParam(r)
	page := w.pageParam(r)
	courses, total, err := w.store.ListCourses(r.Context(), 0, query, page)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	user := w.currentUser(r)
	data := PageData{
		Title:     "Courses",
		Header:    "Courses",
		Action:    "/courses",
		Query:     query,
		Courses:   courses,
		CanManage: user != nil && user.IsSuperAdmin,
	}
	w.fillPagination(&data, page, total)
	w.render(wr, r, http.StatusOK, "courses_index", data)
}

func (w *Web) courseShow(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	course, err := w.store.GetCourse(r.Context(), id)
	if isNotFound(err) {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	exams, err := w.store.ListCourseExams(r.Context(), course.ID)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	user := w.currentUser(r)
	w.render(wr, r, http.StatusOK, "course_show", PageData{
		Title:     course.Name,
		Header:    course.Name,
		Course:    &course,
		Exams:     exams,
		CanManage: user != nil && w.store.UserCanManageCourse(r.Context(), user, course.ID),
	})
}

// manageableSubjects returns the subjects the user may attach courses to.
func (w *Web) manageableSubjects(r *http.Request) ([]models.Subject, error) {
	user := w.currentUser(r)
	if user == nil {
		return nil, nil
	}
	if user.IsSuperAdmin {
		return w.allSubjects(r)
	}
	var out []models.Subject
	for _, sid := range user.SubjectIDs {
		if s, err := w.store.GetSubject(r.Context(), sid); err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

func (w *Web) courseNew(wr http.ResponseWriter, r *http.Request) {
	subjects, err := w.manageableSubjects(r)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.render(wr, r, http.StatusOK, "course_form", PageData{
		Title: "New Course", Header: "Courses", Subjects: subjects,
	})
}

func (w *Web) courseCreate(wr http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.serverError(wr, r, err)
		return
	}
	user := w.currentUser(r)
	name := strings.TrimSpace(r.PostFormValue("name"))
	identifier := strings.TrimSpace(r.PostFormValue("identifier"))
	keywords := strings.TrimSpace(r.PostFormValue("keywords"))
	subjectID, err := w.formInt(r, "subject_id")

	var errs []string
	if name == "" {
		errs = append(errs, "Name can't be blank.")
	}
	if err != nil || subjectID == 0 {
		errs = append(errs, "Subject must be selected.")
	} else if !w.store.UserCanManageSubject(r.Context(), user, subjectID) {
		errs = append(errs, "You are not allowed to manage courses under this subject.")
	}

	subjects, _ := w.manageableSubjects(r)
	if len(errs) > 0 {
		w.renderErrors(wr, r, http.StatusUnprocessableEntity, "course_form",
			PageData{Title: "New Course", Header: "Courses",
				Subjects: subjects, SelSubject: subjectID}, errs)
		return
	}

	course, err := w.store.CreateCourse(r.Context(), name, identifier, subjectID, keywords)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.redirect(wr, r, "/courses/"+itoa(course.ID), "Course was successfully created.")
}

func (w *Web) courseEdit(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	course, err := w.store.GetCourse(r.Context(), id)
	if isNotFound(err) {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	user := w.currentUser(r)
	if !w.store.UserCanManageSubject(r.Context(), user, course.SubjectID) {
		w.redirect(wr, r, "/", "You are not allowed to manage courses under this subject.")
		return
	}
	subjects, err := w.manageableSubjects(r)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.render(wr, r, http.StatusOK, "course_form", PageData{
		Title: "Edit Course", Header: "Courses", Course: &course,
		Subjects: subjects, SelSubject: course.SubjectID,
	})
}

func (w *Web) courseUpdate(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.serverError(wr, r, err)
		return
	}
	course, err := w.store.GetCourse(r.Context(), id)
	if isNotFound(err) {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	user := w.currentUser(r)
	if !w.store.UserCanManageSubject(r.Context(), user, course.SubjectID) {
		w.redirect(wr, r, "/", "You are not allowed to manage courses under this subject.")
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	identifier := strings.TrimSpace(r.PostFormValue("identifier"))
	keywords := strings.TrimSpace(r.PostFormValue("keywords"))
	subjectID, err := w.formInt(r, "subject_id")

	var errs []string
	if name == "" {
		errs = append(errs, "Name can't be blank.")
	}
	if err != nil || subjectID == 0 {
		errs = append(errs, "Subject must be selected.")
	} else if !w.store.UserCanManageSubject(r.Context(), user, subjectID) {
		errs = append(errs, "You are not allowed to manage courses under this subject.")
	}

	subjects, _ := w.manageableSubjects(r)
	if len(errs) > 0 {
		w.renderErrors(wr, r, http.StatusUnprocessableEntity, "course_form",
			PageData{Title: "Edit Course", Header: "Courses", Course: &course,
				Subjects: subjects, SelSubject: subjectID}, errs)
		return
	}

	if err := w.store.UpdateCourse(r.Context(), id, name, identifier, subjectID, keywords); err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.redirect(wr, r, "/courses/"+itoa(id), "Course was successfully updated.")
}

func (w *Web) courseDelete(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	course, err := w.store.GetCourse(r.Context(), id)
	if isNotFound(err) {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	user := w.currentUser(r)
	if !w.store.UserCanManageSubject(r.Context(), user, course.SubjectID) {
		w.redirect(wr, r, "/", "You are not allowed to manage courses under this subject.")
		return
	}
	if err := w.store.DeleteCourse(r.Context(), id); err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.redirect(wr, r, "/courses", "Course was successfully destroyed.")
}

func isNotFound(err error) bool {
	return errors.Is(err, db.ErrNotFound)
}
