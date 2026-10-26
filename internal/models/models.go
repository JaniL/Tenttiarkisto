package models

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/JaniL/tenttiarkisto/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the underlying connection pool (used by the session store).
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Ping checks database connectivity.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

const pageSize = 20

var Languages = []string{"Finnish", "Swedish", "English"}

func LanguageName(i int) string {
	if i >= 0 && i < len(Languages) {
		return Languages[i]
	}
	return "Unknown"
}

type Subject struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Course struct {
	ID         int64
	Name       string
	Identifier string
	Keywords   string
	SubjectID  int64
	Subject    *Subject
	ExamCount  int
}

type User struct {
	ID           int64
	Organization string
	Email        string
	PasswordHash string
	IsSuperAdmin bool
	SubjectIDs   []int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Exam struct {
	ID          int64
	Language    int
	Date        *time.Time
	Description string
	Author      string
	CourseID    int64
	Course      *Course
	Approved    bool
	Rejected    bool
	Filepath    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ---- Subjects ----

func (s *Store) CreateSubject(ctx context.Context, name string) (Subject, error) {
	var sub Subject
	err := s.pool.QueryRow(ctx,
		`INSERT INTO subjects (name) VALUES ($1) RETURNING id, name, created_at, updated_at`,
		name,
	).Scan(&sub.ID, &sub.Name, &sub.CreatedAt, &sub.UpdatedAt)
	return sub, mapErr(err)
}

// CreateSubjectIfMissing inserts a subject unless the name exists.
func (s *Store) CreateSubjectIfMissing(ctx context.Context, name string) (Subject, error) {
	var sub Subject
	err := s.pool.QueryRow(ctx, `
		INSERT INTO subjects (name) VALUES ($1)
		ON CONFLICT DO NOTHING
		RETURNING id, name, created_at, updated_at`,
		name).Scan(&sub.ID, &sub.Name, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) { // conflict path returns no rows
			rows, err2 := s.pool.Query(ctx,
				`SELECT id, name, created_at, updated_at FROM subjects WHERE name = $1`, name)
			if err2 != nil {
				return Subject{}, err2
			}
			defer rows.Close()
			for rows.Next() {
				if err2 := rows.Scan(&sub.ID, &sub.Name, &sub.CreatedAt, &sub.UpdatedAt); err2 != nil {
					return Subject{}, err2
				}
			}
			return sub, rows.Err()
		}
		return Subject{}, err
	}
	return sub, nil
}

func (s *Store) ListSubjects(ctx context.Context, query string, page int) ([]Subject, int, error) {
	where := ""
	args := []any{}
	if q := strings.TrimSpace(strings.ToLower(query)); q != "" {
		args = append(args, "%"+q+"%")
		where = "WHERE LOWER(name) LIKE $1"
	}
	total, err := s.count(ctx, "subjects", where, args)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, created_at, updated_at FROM subjects `+where+`
		ORDER BY LOWER(name) ASC LIMIT $`+nextArg(args, 1)+` OFFSET $`+nextArg(args, 2),
		append(args, pageSize, offset(page))...)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()
	var out []Subject
	for rows.Next() {
		var sub Subject
		if err := rows.Scan(&sub.ID, &sub.Name, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, sub)
	}
	return out, total, rows.Err()
}

func (s *Store) GetSubject(ctx context.Context, id int64) (Subject, error) {
	var sub Subject
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, created_at, updated_at FROM subjects WHERE id = $1`, id,
	).Scan(&sub.ID, &sub.Name, &sub.CreatedAt, &sub.UpdatedAt)
	return sub, mapErr(err)
}

func (s *Store) UpdateSubject(ctx context.Context, id int64, name string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE subjects SET name = $2, updated_at = now() WHERE id = $1`, id, name)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteSubject(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM subjects WHERE id = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

// ---- Courses ----

func (s *Store) CreateCourse(ctx context.Context, name, identifier string, subjectID int64, keywords string) (Course, error) {
	var c Course
	err := s.pool.QueryRow(ctx, `
		INSERT INTO courses (name, identifier, subject_id, keywords)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, identifier, subject_id, keywords`,
		name, identifier, subjectID, keywords,
	).Scan(&c.ID, &c.Name, &c.Identifier, &c.SubjectID, &c.Keywords)
	return c, mapErr(err)
}

func (s *Store) ListCourses(ctx context.Context, subjectID int64, query string, page int) ([]Course, int, error) {
	args := []any{}
	where := []string{}
	if subjectID > 0 {
		args = append(args, subjectID)
		where = append(where, "c.subject_id = $"+itoa(len(args)))
	}
	if q := strings.TrimSpace(strings.ToLower(query)); q != "" {
		args = append(args, "%"+q+"%")
		n := itoa(len(args))
		where = append(where,
			"(LOWER(c.name) LIKE $"+n+" OR LOWER(c.identifier) LIKE $"+n+" OR LOWER(c.keywords) LIKE $"+n+")")
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}
	total, err := s.countJoin(ctx, `courses c LEFT JOIN subjects s ON s.id = c.subject_id`, whereSQL, args)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.name, c.identifier, c.subject_id, c.keywords,
		       s.id, s.name,
		       (SELECT count(*) FROM exams e WHERE e.course_id = c.id)
		FROM courses c LEFT JOIN subjects s ON s.id = c.subject_id `+whereSQL+`
		ORDER BY LOWER(c.name) ASC LIMIT $`+nextArg(args, 1)+` OFFSET $`+nextArg(args, 2),
		append(args, pageSize, offset(page))...)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()
	var out []Course
	for rows.Next() {
		var c Course
		var subID *int64
		var subName *string
		if err := rows.Scan(&c.ID, &c.Name, &c.Identifier, &c.SubjectID, &c.Keywords,
			&subID, &subName, &c.ExamCount); err != nil {
			return nil, 0, err
		}
		if subID != nil && subName != nil {
			c.Subject = &Subject{ID: *subID, Name: *subName}
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (s *Store) GetCourse(ctx context.Context, id int64) (Course, error) {
	var c Course
	var subID *int64
	var subName *string
	err := s.pool.QueryRow(ctx, `
		SELECT c.id, c.name, c.identifier, c.subject_id, c.keywords,
		       s.id, s.name,
		       (SELECT count(*) FROM exams e WHERE e.course_id = c.id)
		FROM courses c LEFT JOIN subjects s ON s.id = c.subject_id
		WHERE c.id = $1`, id,
	).Scan(&c.ID, &c.Name, &c.Identifier, &c.SubjectID, &c.Keywords,
		&subID, &subName, &c.ExamCount)
	if subID != nil && subName != nil {
		c.Subject = &Subject{ID: *subID, Name: *subName}
	}
	return c, mapErr(err)
}

func (s *Store) UpdateCourse(ctx context.Context, id int64, name, identifier string, subjectID int64, keywords string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE courses SET name = $2, identifier = $3, subject_id = $4, keywords = $5, updated_at = now()
		WHERE id = $1`, id, name, identifier, subjectID, keywords)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteCourse(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM courses WHERE id = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

// ---- Users ----

func (s *Store) CreateUser(ctx context.Context, organization, email, passwordHash string, isSuperAdmin bool, subjectIDs []int64) (User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var u User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (organization, email, encrypted_password, is_super_admin)
		VALUES ($1, $2, $3, $4)
		RETURNING id, organization, email, encrypted_password, is_super_admin`,
		organization, strings.ToLower(email), passwordHash, isSuperAdmin,
	).Scan(&u.ID, &u.Organization, &u.Email, &u.PasswordHash, &u.IsSuperAdmin)
	if err != nil {
		return User{}, mapErr(err)
	}
	if err := setUserSubjects(ctx, tx, u.ID, subjectIDs); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	u.SubjectIDs = subjectIDs
	return u, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.organization, u.email, u.is_super_admin,
		       COALESCE(array_agg(us.subject_id) FILTER (WHERE us.subject_id IS NOT NULL), '{}')
		FROM users u LEFT JOIN subjects_users us ON us.user_id = u.id
		GROUP BY u.id
		ORDER BY LOWER(u.organization) ASC`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var subj []int64
		if err := rows.Scan(&u.ID, &u.Organization, &u.Email, &u.IsSuperAdmin, &subj); err != nil {
			return nil, err
		}
		u.SubjectIDs = subj
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) GetUser(ctx context.Context, id int64) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.organization, u.email, u.encrypted_password, u.is_super_admin,
		       COALESCE((SELECT array_agg(us.subject_id) FROM subjects_users us WHERE us.user_id = u.id), '{}'),
		       u.created_at, u.updated_at
		FROM users u WHERE u.id = $1`, id,
	).Scan(&u.ID, &u.Organization, &u.Email, &u.PasswordHash, &u.IsSuperAdmin, &u.SubjectIDs, &u.CreatedAt, &u.UpdatedAt)
	return u, mapErr(err)
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.organization, u.email, u.encrypted_password, u.is_super_admin,
		       COALESCE((SELECT array_agg(us.subject_id) FROM subjects_users us WHERE us.user_id = u.id), '{}'),
		       u.created_at, u.updated_at
		FROM users u WHERE u.email = $1`, strings.ToLower(strings.TrimSpace(email)),
	).Scan(&u.ID, &u.Organization, &u.Email, &u.PasswordHash, &u.IsSuperAdmin, &u.SubjectIDs, &u.CreatedAt, &u.UpdatedAt)
	return u, mapErr(err)
}

func (s *Store) UpdateUser(ctx context.Context, id int64, organization, email string, passwordHash *string, subjectIDs []int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var tag pgconn.CommandTag
	if passwordHash == nil {
		tag, err = tx.Exec(ctx, `
			UPDATE users SET organization = $2, email = $3, updated_at = now() WHERE id = $1`,
			id, organization, strings.ToLower(email))
	} else {
		tag, err = tx.Exec(ctx, `
			UPDATE users SET organization = $2, email = $3, encrypted_password = $4, updated_at = now() WHERE id = $1`,
			id, organization, strings.ToLower(email), *passwordHash)
	}
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM subjects_users WHERE user_id = $1`, id); err != nil {
		return err
	}
	if err := setUserSubjects(ctx, tx, id, subjectIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

// ---- Exams ----

func (s *Store) CreateExam(ctx context.Context, e *Exam) error {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO exams (language, date, description, author, course_id, approved, rejected, filepath)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`,
		e.Language, e.Date, e.Description, e.Author, e.CourseID, e.Approved, e.Rejected, e.Filepath,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
	return mapErr(err)
}

type examFilter struct {
	where  string
	args   []any
	course bool
}

func (s *Store) listExams(ctx context.Context, f examFilter) ([]Exam, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.language, e.date, e.description, e.author, e.course_id,
		       e.approved, e.rejected, e.filepath,
		       c.id, c.name, s.id, s.name
		FROM exams e
		JOIN courses c ON c.id = e.course_id
		JOIN subjects s ON s.id = c.subject_id
		`+f.where+`
		ORDER BY e.date DESC NULLS LAST, e.id DESC
		LIMIT 100`, f.args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []Exam
	for rows.Next() {
		var e Exam
		var c Course
		var sub Subject
		if err := rows.Scan(&e.ID, &e.Language, &e.Date, &e.Description, &e.Author, &e.CourseID,
			&e.Approved, &e.Rejected, &e.Filepath,
			&c.ID, &c.Name, &sub.ID, &sub.Name); err != nil {
			return nil, err
		}
		c.Subject = &sub
		e.Course = &c
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListPublicExams returns approved exams (public index).
func (s *Store) ListPublicExams(ctx context.Context) ([]Exam, error) {
	return s.listExams(ctx, examFilter{where: "WHERE e.approved = true"})
}

// ListCourseExams returns approved exams for a course (public course page).
func (s *Store) ListCourseExams(ctx context.Context, courseID int64) ([]Exam, error) {
	return s.listExams(ctx, examFilter{
		where: "WHERE e.approved = true AND e.course_id = $1",
		args:  []any{courseID},
	})
}

// ListPendingExams returns unapproved exams. When subjectIDs is nil
// (super admin) all pending exams are returned; otherwise only exams
// within the given subjects.
func (s *Store) ListPendingExams(ctx context.Context, subjectIDs []int64) ([]Exam, error) {
	if subjectIDs == nil {
		return s.listExams(ctx, examFilter{where: "WHERE NOT e.approved"})
	}
	if len(subjectIDs) == 0 {
		return []Exam{}, nil
	}
	return s.listExams(ctx, examFilter{
		where: "WHERE NOT e.approved AND s.id = ANY($1)",
		args:  []any{subjectIDs},
	})
}

func (s *Store) GetExam(ctx context.Context, id int64) (Exam, error) {
	var e Exam
	var c Course
	var sub Subject
	err := s.pool.QueryRow(ctx, `
		SELECT e.id, e.language, e.date, e.description, e.author, e.course_id,
		       e.approved, e.rejected, e.filepath,
		       c.id, c.name, s.id, s.name
		FROM exams e
		JOIN courses c ON c.id = e.course_id
		JOIN subjects s ON s.id = c.subject_id
		WHERE e.id = $1`, id,
	).Scan(&e.ID, &e.Language, &e.Date, &e.Description, &e.Author, &e.CourseID,
		&e.Approved, &e.Rejected, &e.Filepath,
		&c.ID, &c.Name, &sub.ID, &sub.Name)
	if err != nil {
		return e, mapErr(err)
	}
	c.Subject = &sub
	e.Course = &c
	return e, nil
}

func (s *Store) UpdateExam(ctx context.Context, e *Exam) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE exams SET language = $2, date = $3, description = $4, author = $5,
		                 course_id = $6, filepath = $7, updated_at = now()
		WHERE id = $1`,
		e.ID, e.Language, e.Date, e.Description, e.Author, e.CourseID, e.Filepath)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (s *Store) SetExamApproval(ctx context.Context, id int64, approved, rejected bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE exams SET approved = $2, rejected = $3, updated_at = now() WHERE id = $1`,
		id, approved, rejected)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteExam(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM exams WHERE id = $1`, id)
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (e Exam) LanguageStr() string { return LanguageName(e.Language) }

func (e Exam) DateStr() string {
	if e.Date == nil {
		return ""
	}
	return e.Date.Format("2006-01-02")
}

// ---- Permissions (ported from ActiveRecord models) ----

func (s *Store) UserCanManageSubject(ctx context.Context, u *User, subjectID int64) bool {
	if u == nil {
		return false
	}
	if u.IsSuperAdmin {
		return true
	}
	return contains(u.SubjectIDs, subjectID)
}

func (s *Store) UserCanManageCourse(ctx context.Context, u *User, courseID int64) bool {
	if u == nil {
		return false
	}
	c, err := s.GetCourse(ctx, courseID)
	if err != nil {
		return false
	}
	return s.UserCanManageSubject(ctx, u, c.SubjectID)
}

// ---- helpers ----

func setUserSubjects(ctx context.Context, tx pgx.Tx, userID int64, subjectIDs []int64) error {
	for _, sid := range subjectIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO subjects_users (user_id, subject_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			userID, sid); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) count(ctx context.Context, table, where string, args []any) (int, error) {
	var total int
	q := `SELECT count(*) FROM ` + table
	if where != "" {
		q += " " + where
	}
	err := s.pool.QueryRow(ctx, q, args...).Scan(&total)
	return total, mapErr(err)
}

func (s *Store) countJoin(ctx context.Context, from, where string, args []any) (int, error) {
	var total int
	q := `SELECT count(*) FROM ` + from
	if where != "" {
		q += " " + where
	}
	err := s.pool.QueryRow(ctx, q, args...).Scan(&total)
	return total, mapErr(err)
}

func contains(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func offset(page int) int {
	if page < 1 {
		page = 1
	}
	return (page - 1) * pageSize
}

func itoa(n int) string { return strconv.Itoa(n) }

func nextArg(args []any, extra int) string {
	return strconv.Itoa(len(args) + extra)
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if err == pgx.ErrNoRows {
		return db.ErrNotFound
	}
	return err
}
