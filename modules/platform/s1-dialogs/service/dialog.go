package service

import "system1/platform/s1-dialogs/store/sqlc"

type Dialog struct {
	q *sqlc.Queries
}

func NewDialog(q *sqlc.Queries) *Dialog {
	return &Dialog{q: q}
}
