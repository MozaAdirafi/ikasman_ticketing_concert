package service

import (
	"context"

	db "github.com/MozaAdirafi/ikasman_ticketing_concert/internal/db/sqlc"
)

type TicketService struct {
	q *db.Queries
}

func NewTicketService(q *db.Queries) *TicketService {
	return &TicketService{q: q}
}

func (s *TicketService) ListTickets(ctx context.Context) ([]db.Ticket, error) {
	tickets, err := s.q.ListTickets(ctx)
	if err != nil {
		return nil, err
	}
	return tickets, nil
}
