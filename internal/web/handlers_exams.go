package web

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/JaniL/tenttiarkisto/internal/models"
	"github.com/JaniL/tenttiarkisto/internal/pdfmerge"
	"github.com/JaniL/tenttiarkisto/internal/storage"
)

const maxUploadBytes = 64 << 20 // 64 MB of images per exam

func (w *Web) examsIndex(wr http.ResponseWriter, r *http.Request) {
	exams, err := w.store.ListPublicExams(r.Context())
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.render(wr, r, http.StatusOK, "exams_index", PageData{
		Title: "Exams", Header: "Exams", Exams: exams,
	})
}

func (w *Web) approvelisting(wr http.ResponseWriter, r *http.Request) {
	user := w.currentUser(r)
	var subjectIDs []int64
	if !user.IsSuperAdmin {
		subjectIDs = user.SubjectIDs
	}
	exams, err := w.store.ListPendingExams(r.Context(), subjectIDs)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.render(wr, r, http.StatusOK, "approvelisting", PageData{
		Title: "Approve Exams", Header: "Approve Exams", Exams: exams,
	})
}

func (w *Web) examShow(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	exam, err := w.store.GetExam(r.Context(), id)
	if isNotFound(err) {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	user := w.currentUser(r)
	w.render(wr, r, http.StatusOK, "exam_show", PageData{
		Title:     "Exam",
		Header:    "Exam",
		Exam:      &exam,
		FileName:  path.Base(exam.Filepath),
		CanManage: user != nil && w.store.UserCanManageSubject(r.Context(), user, exam.Course.SubjectID),
	})
}

// examFile streams the exam PDF from object storage.
func (w *Web) examFile(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	exam, err := w.store.GetExam(r.Context(), id)
	if isNotFound(err) {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	if exam.Filepath == "" {
		w.notFound(wr, r)
		return
	}
	obj, err := w.s3.Get(r.Context(), exam.Filepath)
	if err == storage.ErrNotFound {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	defer obj.Close()

	wr.Header().Set("Content-Type", "application/pdf")
	wr.Header().Set("Content-Disposition",
		fmt.Sprintf(`inline; filename=%q`, path.Base(exam.Filepath)))
	wr.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(wr, obj)
}

// examFormData loads the data needed by the exam form.
func (w *Web) examFormData(r *http.Request) ([]models.Course, error) {
	courses, _, err := w.store.ListCourses(r.Context(), 0, "", 1)
	if err != nil {
		return nil, err
	}
	// fetch all pages (admin tool / exam creation)
	for page := 2; ; page++ {
		more, total, err := w.store.ListCourses(r.Context(), 0, "", page)
		if err != nil {
			return nil, err
		}
		if len(more) == 0 {
			break
		}
		courses = append(courses, more...)
		if page*pageSize >= total {
			break
		}
	}
	return courses, nil
}

func (w *Web) examNew(wr http.ResponseWriter, r *http.Request) {
	courses, err := w.examFormData(r)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.render(wr, r, http.StatusOK, "exam_form", PageData{
		Title: "New Exam", Header: "New Exam", Courses: courses, Exam: &models.Exam{},
	})
}

// examForm holds the parsed exam form fields.
type examForm struct {
	Language    int
	Date        *time.Time
	DateStr     string
	Description string
	Author      string
	CourseID    int64
	Images      [][]byte
}

func (w *Web) parseExamForm(r *http.Request) (examForm, []string) {
	var form examForm
	var errs []string

	// bound upload size
	r.Body = http.MaxBytesReader(nil, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		errs = append(errs, "Invalid form submission (uploads max 64 MB).")
		return form, errs
	}

	lang := r.PostFormValue("language")
	switch lang {
	case "Finnish":
		form.Language = 0
	case "Swedish":
		form.Language = 1
	case "English":
		form.Language = 2
	default:
		errs = append(errs, "Language must be selected.")
	}

	dateStr := strings.TrimSpace(r.PostFormValue("date"))
	form.DateStr = dateStr
	if dateStr == "" {
		errs = append(errs, "Date can't be blank.")
	} else {
		d, err := formDate(dateStr)
		if err != nil {
			errs = append(errs, "Date is invalid.")
		} else {
			form.Date = d
		}
	}

	form.Description = strings.TrimSpace(r.PostFormValue("description"))
	form.Author = strings.TrimSpace(r.PostFormValue("author"))

	courseID, err := w.formInt(r, "course_id")
	if err != nil || courseID == 0 {
		errs = append(errs, "Course must be selected.")
	}
	form.CourseID = courseID

	if r.MultipartForm != nil {
		for _, fh := range r.MultipartForm.File["files"] {
			f, err := fh.Open()
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(f)
			f.Close()
			if len(data) == 0 {
				continue
			}
			form.Images = append(form.Images, data)
		}
	}
	return form, errs
}

func (w *Web) examCreate(wr http.ResponseWriter, r *http.Request) {
	user := w.currentUser(r)
	form, errs := w.parseExamForm(r)

	if form.CourseID != 0 {
		if !w.store.UserCanManageCourse(r.Context(), user, form.CourseID) {
			errs = append(errs, "You are not allowed to manage exams for this course.")
		}
	}
	if len(form.Images) == 0 {
		errs = append(errs, "At least one image file is required.")
	}

	if len(errs) > 0 {
		courses, _ := w.examFormData(r)
		w.renderErrors(wr, r, http.StatusUnprocessableEntity, "exam_form",
			PageData{Title: "New Exam", Header: "New Exam", Courses: courses,
				Exam: formToExam(form)}, errs)
		return
	}

	key, err := w.uploadExamPDF(r.Context(), form)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}

	exam := &models.Exam{
		Language:    form.Language,
		Date:        form.Date,
		Description: form.Description,
		Author:      form.Author,
		CourseID:    form.CourseID,
		Filepath:    key,
	}
	if err := w.store.CreateExam(r.Context(), exam); err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.redirect(wr, r, "/exams/"+itoa(exam.ID),
		"Exam was successfully created and will show up once approved.")
}

func (w *Web) examEdit(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	exam, err := w.store.GetExam(r.Context(), id)
	if isNotFound(err) {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	user := w.currentUser(r)
	if !w.store.UserCanManageSubject(r.Context(), user, exam.Course.SubjectID) {
		w.redirect(wr, r, "/", "You are not allowed to manage exams for this course.")
		return
	}
	courses, err := w.examFormData(r)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.render(wr, r, http.StatusOK, "exam_form", PageData{
		Title: "Edit Exam", Header: "Edit Exam", Exam: &exam, Courses: courses,
	})
}

func (w *Web) examUpdate(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	exam, err := w.store.GetExam(r.Context(), id)
	if isNotFound(err) {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	user := w.currentUser(r)
	if !w.store.UserCanManageSubject(r.Context(), user, exam.Course.SubjectID) {
		w.redirect(wr, r, "/", "You are not allowed to manage exams for this course.")
		return
	}

	form, errs := w.parseExamForm(r)
	if form.CourseID != 0 &&
		!w.store.UserCanManageSubject(r.Context(), user, form.CourseID) &&
		!w.store.UserCanManageCourse(r.Context(), user, form.CourseID) {
		errs = append(errs, "You are not allowed to manage exams for this course.")
	}

	oldKey := exam.Filepath

	if len(form.Images) > 0 {
		key, err := w.uploadExamPDF(r.Context(), form)
		if err != nil {
			w.serverError(wr, r, err)
			return
		}
		exam.Filepath = key
	}

	if len(errs) > 0 {
		courses, _ := w.examFormData(r)
		if len(form.Images) == 0 {
			exam.Filepath = oldKey
		}
		w.renderErrors(wr, r, http.StatusUnprocessableEntity, "exam_form",
			PageData{Title: "Edit Exam", Header: "Edit Exam", Exam: &exam, Courses: courses}, errs)
		return
	}

	exam.Language = form.Language
	exam.Date = form.Date
	exam.Description = form.Description
	exam.Author = form.Author
	exam.CourseID = form.CourseID
	if err := w.store.UpdateExam(r.Context(), &exam); err != nil {
		w.serverError(wr, r, err)
		return
	}

	// delete replaced PDF from object storage
	if len(form.Images) > 0 && oldKey != "" && oldKey != exam.Filepath {
		if err := w.s3.Delete(r.Context(), oldKey); err != nil {
			w.logger.Warn("failed to delete old exam pdf", "key", oldKey, "err", err)
		}
	}
	w.redirect(wr, r, "/exams/"+itoa(exam.ID), "Exam was successfully updated.")
}

func (w *Web) examDelete(wr http.ResponseWriter, r *http.Request) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	exam, err := w.store.GetExam(r.Context(), id)
	if isNotFound(err) {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	user := w.currentUser(r)
	if !w.store.UserCanManageSubject(r.Context(), user, exam.Course.SubjectID) {
		w.redirect(wr, r, "/", "You are not allowed to manage exams for this course.")
		return
	}
	if exam.Filepath != "" {
		if err := w.s3.Delete(r.Context(), exam.Filepath); err != nil {
			w.logger.Warn("failed to delete exam pdf", "key", exam.Filepath, "err", err)
		}
	}
	if err := w.store.DeleteExam(r.Context(), id); err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.redirect(wr, r, "/exams", "Exam was successfully destroyed.")
}

func (w *Web) examApprove(wr http.ResponseWriter, r *http.Request) {
	w.setApproval(wr, r, true)
}

func (w *Web) examReject(wr http.ResponseWriter, r *http.Request) {
	w.setApproval(wr, r, false)
}

func (w *Web) setApproval(wr http.ResponseWriter, r *http.Request, approve bool) {
	id, err := w.pathID(r)
	if err != nil {
		w.notFound(wr, r)
		return
	}
	exam, err := w.store.GetExam(r.Context(), id)
	if isNotFound(err) {
		w.notFound(wr, r)
		return
	} else if err != nil {
		w.serverError(wr, r, err)
		return
	}
	user := w.currentUser(r)
	if !w.store.UserCanManageSubject(r.Context(), user, exam.Course.SubjectID) {
		w.redirect(wr, r, "/", "You are not allowed to manage exams for this course.")
		return
	}
	if err := w.store.SetExamApproval(r.Context(), id, approve, !approve); err != nil {
		w.serverError(wr, r, err)
		return
	}
	if approve {
		w.redirect(wr, r, "/approvelisting", "Exam approved.")
	} else {
		w.redirect(wr, r, "/approvelisting", "Exam rejected.")
	}
}

// uploadExamPDF merges the uploaded images into one PDF, stores it in
// the bucket and returns its key.
func (w *Web) uploadExamPDF(ctx context.Context, form examForm) (string, error) {
	pdf, err := pdfmerge.ImageToPDF(form.Images)
	if err != nil {
		return "", err
	}
	course, err := w.store.GetCourse(ctx, form.CourseID)
	if err != nil {
		return "", err
	}
	date := "undated"
	if form.Date != nil {
		date = form.Date.UTC().Format("2006-01-02")
	}
	key := fmt.Sprintf("exams/%d/%s_%s.pdf", course.ID, date, slugify(course.Name))
	if err := w.s3.Put(ctx, key, bytes.NewReader(pdf), int64(len(pdf)),
		"application/pdf"); err != nil {
		return "", err
	}
	return key, nil
}

func formToExam(form examForm) *models.Exam {
	return &models.Exam{
		Language:    form.Language,
		Date:        form.Date,
		Description: form.Description,
		Author:      form.Author,
		CourseID:    form.CourseID,
	}
}
