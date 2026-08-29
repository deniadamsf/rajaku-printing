// Implementasi orderapi.CustomerOrderReader — jembatan lintas modul yang
// dipakai fitur "Manajemen Pelanggan" (modul auth) untuk membaca statistik &
// riwayat order singkat seorang pelanggan tanpa modul auth pernah mengimport
// package internal order (§22). File terpisah dari order_service.go per §22
// (satu tanggung jawab per file).
package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/rajaku-printing/backend/internal/order/orderapi"
)

// Compile-time assertion — Service satisfies the cross-module read contract.
var _ orderapi.CustomerOrderReader = (*Service)(nil)

// CustomerOrderOverview implements orderapi.CustomerOrderReader.
func (s *Service) CustomerOrderOverview(ctx context.Context, customerID uuid.UUID, recentLimit int) (*orderapi.CustomerOrderOverview, error) {
	stats, err := s.orders.CustomerOrderStats(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("customer order overview: stats for %s: %w", customerID, err)
	}
	recent, err := s.orders.RecentOrdersByCustomer(ctx, customerID, recentLimit)
	if err != nil {
		return nil, fmt.Errorf("customer order overview: recent orders for %s: %w", customerID, err)
	}

	out := &orderapi.CustomerOrderOverview{
		Stats: orderapi.CustomerOrderStats{
			TotalOrders:     stats.TotalOrders,
			CompletedOrders: stats.CompletedOrders,
			CancelledOrders: stats.CancelledOrders,
			TotalSpend:      stats.TotalSpend,
			LastOrderAt:     stats.LastOrderAt,
		},
		Recent: make([]orderapi.CustomerOrderBrief, 0, len(recent)),
	}
	for _, r := range recent {
		out.Recent = append(out.Recent, orderapi.CustomerOrderBrief{
			Resi:      r.Resi,
			Status:    r.Status,
			Channel:   r.Channel,
			Total:     r.Total,
			CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}
