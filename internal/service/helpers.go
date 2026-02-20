package service

import "database/sql"

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func nullInt32(n int) sql.NullInt32 {
	return sql.NullInt32{Int32: int32(n), Valid: n != 0}
}

func nullFloat64(f float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: f, Valid: f != 0}
}
