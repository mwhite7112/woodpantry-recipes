package service

import (
	"database/sql"

	"github.com/mwhite7112/woodpantry-recipes/internal/db"
)

// Service holds dependencies for recipe business logic.
type Service struct {
	q             *db.Queries
	sqlDB         *sql.DB
	dictionaryURL string
	openaiKey     string
	extractModel  string
}

func New(q *db.Queries, sqlDB *sql.DB, dictionaryURL, openaiKey, extractModel string) *Service {
	return &Service{
		q:             q,
		sqlDB:         sqlDB,
		dictionaryURL: dictionaryURL,
		openaiKey:     openaiKey,
		extractModel:  extractModel,
	}
}

func (s *Service) Queries() *db.Queries { return s.q }
func (s *Service) DB() *sql.DB          { return s.sqlDB }
