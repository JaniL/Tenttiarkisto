package web

import (
	"net/http"
	"strings"

	"github.com/JaniL/tenttiarkisto/internal/models"
	"golang.org/x/crypto/bcrypt"
)

func (w *Web) loginForm(wr http.ResponseWriter, r *http.Request) {
	if w.currentUser(r) != nil {
		http.Redirect(wr, r, "/", http.StatusSeeOther)
		return
	}
	w.render(wr, r, http.StatusOK, "login", PageData{Title: "Log in", Header: "Log in"})
}

func (w *Web) loginSubmit(wr http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.serverError(wr, r, err)
		return
	}
	email := strings.TrimSpace(strings.ToLower(r.PostFormValue("email")))
	password := r.PostFormValue("password")

	u, err := w.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		w.logger.Warn("failed login", "email", email, "err", err)
		w.sessions.Put(r.Context(), "errors", []string{"Invalid email or password."})
		w.render(wr, r, http.StatusUnprocessableEntity, "login", PageData{Title: "Log in", Header: "Log in"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		w.logger.Warn("failed login (password)", "email", email)
		w.sessions.Put(r.Context(), "errors", []string{"Invalid email or password."})
		w.render(wr, r, http.StatusUnprocessableEntity, "login", PageData{Title: "Log in", Header: "Log in"})
		return
	}

	w.sessions.Put(r.Context(), "uid", u.ID)
	w.redirect(wr, r, "/", "Logged in successfully.")
}

func (w *Web) registerForm(wr http.ResponseWriter, r *http.Request) {
	if w.currentUser(r) != nil {
		http.Redirect(wr, r, "/", http.StatusSeeOther)
		return
	}
	w.render(wr, r, http.StatusOK, "register", PageData{Title: "Sign up", Header: "Sign up"})
}

func (w *Web) registerSubmit(wr http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.serverError(wr, r, err)
		return
	}
	org := strings.TrimSpace(r.PostFormValue("organization"))
	email := strings.ToLower(strings.TrimSpace(r.PostFormValue("email")))
	password := r.PostFormValue("password")
	confirmation := r.PostFormValue("password_confirmation")

	var errs []string
	if org == "" {
		errs = append(errs, "Organization can't be blank.")
	}
	if !strings.Contains(email, "@") || email == "" {
		errs = append(errs, "Email is invalid.")
	}
	if len(password) < 8 {
		errs = append(errs, "Password must be at least 8 characters.")
	}
	if password != confirmation {
		errs = append(errs, "Password confirmation doesn't match.")
	}
	if len(errs) > 0 {
		w.renderErrors(wr, r, http.StatusUnprocessableEntity, "register",
			PageData{Title: "Sign up", Header: "Sign up"}, errs)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		w.serverError(wr, r, err)
		return
	}
	u, err := w.store.CreateUser(r.Context(), org, email, string(hash), false, nil)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique") {
			w.renderErrors(wr, r, http.StatusUnprocessableEntity, "register",
				PageData{Title: "Sign up", Header: "Sign up"},
				[]string{"Email has already been taken."})
			return
		}
		w.serverError(wr, r, err)
		return
	}

	w.sessions.Put(r.Context(), "uid", u.ID)
	w.redirect(wr, r, "/", "Welcome! Your account has been created.")
}

func (w *Web) logout(wr http.ResponseWriter, r *http.Request) {
	if err := w.sessions.Destroy(r.Context()); err != nil {
		w.serverError(wr, r, err)
		return
	}
	http.Redirect(wr, r, "/", http.StatusSeeOther)
}

func (w *Web) accountForm(wr http.ResponseWriter, r *http.Request) {
	user := w.currentUser(r)
	if user == nil {
		http.Redirect(wr, r, "/login", http.StatusSeeOther)
		return
	}
	var subjects []models.Subject
	if user.IsSuperAdmin {
		var err error
		subjects, _, err = w.store.ListSubjects(r.Context(), "", 1)
		if err != nil {
			w.serverError(wr, r, err)
			return
		}
	}
	var userSubjects []models.Subject
	for _, sid := range user.SubjectIDs {
		if s, err := w.store.GetSubject(r.Context(), sid); err == nil {
			userSubjects = append(userSubjects, s)
		}
	}
	w.render(wr, r, http.StatusOK, "account", PageData{
		Title:        "Account",
		Header:       "Account",
		UserSubjects: userSubjects,
		Subjects:     subjects,
	})
}

func (w *Web) accountSubmit(wr http.ResponseWriter, r *http.Request) {
	user := w.currentUser(r)
	if user == nil {
		http.Redirect(wr, r, "/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.serverError(wr, r, err)
		return
	}
	org := strings.TrimSpace(r.PostFormValue("organization"))
	email := strings.ToLower(strings.TrimSpace(r.PostFormValue("email")))
	newPassword := r.PostFormValue("password")
	currentPassword := r.PostFormValue("current_password")

	var errs []string
	if org == "" {
		errs = append(errs, "Organization can't be blank.")
	}
	if email == "" || !strings.Contains(email, "@") {
		errs = append(errs, "Email is invalid.")
	}

	var hash *string
	if newPassword != "" {
		if len(newPassword) < 8 {
			errs = append(errs, "New password must be at least 8 characters.")
		}
		if err := bcrypt.CompareHashAndPassword(
			[]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
			errs = append(errs, "Current password is required to change the password.")
		}
		if len(errs) == 0 {
			h, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
			if err != nil {
				w.serverError(wr, r, err)
				return
			}
			hs := string(h)
			hash = &hs
		}
	}
	if len(errs) > 0 {
		w.sessions.Put(r.Context(), "errors", errs)
		w.render(wr, r, http.StatusUnprocessableEntity, "account",
			PageData{Title: "Account", Header: "Account"})
		return
	}

	if err := w.store.UpdateUser(r.Context(), user.ID, org, email, hash, user.SubjectIDs); err != nil {
		w.serverError(wr, r, err)
		return
	}
	w.redirect(wr, r, "/account", "Account updated.")
}
