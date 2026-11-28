package service

import "system1/platform/s1-channels-gateway/store/sqlc"

type Channel struct {
	q *sqlc.Queries
}

func NewChannel(q *sqlc.Queries) *Channel {
	return &Channel{q: q}
}
