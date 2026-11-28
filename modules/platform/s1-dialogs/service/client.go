package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	dialogsapi "s1_dialogs_api/models"
	"system1/platform/internal/pg"
	"system1/platform/s1-dialogs/store/sqlc"
)

type Client struct {
	q *sqlc.Queries
}

func NewClient(q *sqlc.Queries) *Client {
	return &Client{q: q}
}

func (s *Client) Get(ctx context.Context, clientID, serviceID uuid.UUID) (dialogsapi.ServiceClientDto, error) {
	row, err := s.q.GetServiceClient(ctx, sqlc.GetServiceClientParams{
		ClientID:  clientID,
		ServiceID: serviceID,
	})
	if err != nil {
		return dialogsapi.ServiceClientDto{}, fmt.Errorf("get service client: %w", pg.MapErr(err))
	}
	return serviceClientDto(row), nil
}

func serviceClientDto(row sqlc.GetServiceClientRow) dialogsapi.ServiceClientDto {
	return dialogsapi.ServiceClientDto{
		ClientId:    row.ClientID.String(),
		ServiceId:   row.ServiceID.String(),
		DisplayName: row.DisplayName,
		Language:    deref(row.Language),
		Phone:       deref(row.Phone),
		Email:       deref(row.Email),
		Blocked:     row.Blocked,
		Restricted:  row.Restricted,
		Created:     row.Created,
	}
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
