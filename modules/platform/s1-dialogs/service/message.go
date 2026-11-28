package service

import "system1/platform/s1-dialogs/store/sqlc"

type Message struct {
	q *sqlc.Queries
}

func NewMessage(q *sqlc.Queries) *Message {
	return &Message{q: q}
}
