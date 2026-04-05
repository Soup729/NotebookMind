package service

import (
	"context"
	"fmt"
	"time"

	"enterprise-pdf-ai/internal/repository"
)

type DashboardOverview struct {
	TotalDocuments     int64              `json:"total_documents"`
	CompletedDocuments int64              `json:"completed_documents"`
	TotalSessions      int64              `json:"total_sessions"`
	TotalMessages      int64              `json:"total_messages"`
	TotalTokens        int64              `json:"total_tokens"`
	DailyTokens        []DailyTokenMetric `json:"daily_tokens"`
}

type UsageSummary struct {
	TotalTokens int64              `json:"total_tokens"`
	DailyTokens []DailyTokenMetric `json:"daily_tokens"`
}

type DailyTokenMetric struct {
	Date   string `json:"date"`
	Tokens int64  `json:"tokens"`
}

type DashboardService interface {
	GetOverview(ctx context.Context, userID string) (*DashboardOverview, error)
	GetUsageSummary(ctx context.Context, userID string) (*UsageSummary, error)
}

type dashboardService struct {
	documents repository.DocumentRepository
	chats     repository.ChatRepository
}

func NewDashboardService(documents repository.DocumentRepository, chats repository.ChatRepository) DashboardService {
	return &dashboardService{
		documents: documents,
		chats:     chats,
	}
}

func (s *dashboardService) GetOverview(ctx context.Context, userID string) (*DashboardOverview, error) {
	totalDocuments, err := s.documents.CountByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("count documents: %w", err)
	}
	completedDocuments, err := s.documents.CountCompletedByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("count completed documents: %w", err)
	}
	totalSessions, err := s.chats.CountSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("count sessions: %w", err)
	}
	totalMessages, err := s.chats.CountMessages(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("count messages: %w", err)
	}
	totalTokens, err := s.chats.SumTokens(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("sum tokens: %w", err)
	}
	dailyRows, err := s.chats.DailyTokenUsage(ctx, userID, 7)
	if err != nil {
		return nil, fmt.Errorf("load daily usage: %w", err)
	}

	return &DashboardOverview{
		TotalDocuments:     totalDocuments,
		CompletedDocuments: completedDocuments,
		TotalSessions:      totalSessions,
		TotalMessages:      totalMessages,
		TotalTokens:        totalTokens,
		DailyTokens:        normalizeDailyUsage(dailyRows, 7),
	}, nil
}

func (s *dashboardService) GetUsageSummary(ctx context.Context, userID string) (*UsageSummary, error) {
	totalTokens, err := s.chats.SumTokens(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("sum tokens: %w", err)
	}
	dailyRows, err := s.chats.DailyTokenUsage(ctx, userID, 14)
	if err != nil {
		return nil, fmt.Errorf("load daily usage: %w", err)
	}
	return &UsageSummary{
		TotalTokens: totalTokens,
		DailyTokens: normalizeDailyUsage(dailyRows, 14),
	}, nil
}

func normalizeDailyUsage(rows []repository.DailyUsageRow, days int) []DailyTokenMetric {
	byDate := make(map[string]int64, len(rows))
	for _, row := range rows {
		byDate[row.Day.Format("2006-01-02")] = row.Tokens
	}

	metrics := make([]DailyTokenMetric, 0, days)
	start := time.Now().AddDate(0, 0, -days+1)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		metrics = append(metrics, DailyTokenMetric{
			Date:   day,
			Tokens: byDate[day],
		})
	}
	return metrics
}
