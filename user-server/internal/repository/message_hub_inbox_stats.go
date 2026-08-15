package repository

import (
	"context"

	"time"

	"hivemtk-user/internal/model"
)

type HubStatsResult struct {
	Total       int64
	Inbound     int64
	Outbound    int64
	Unread      int64
	ByPlatform  map[string]int64
	ByDirection map[string]int64
	ByMsgType   map[string]int64
	ByAccount   map[string]int64
	Recent24h   int64
}

func (r *MessageHubRepository) GetHubStats(ctx context.Context, start, end *time.Time) (*HubStatsResult, error) {
	if r == nil || r.db == nil {
		return &HubStatsResult{
			ByPlatform: map[string]int64{}, ByDirection: map[string]int64{},
			ByMsgType: map[string]int64{}, ByAccount: map[string]int64{},
		}, nil
	}

	var total, inbound, outbound, unread int64

	countQuery := r.db.WithContext(ctx).Model(&model.MessageHub{}).Where("1 = 1")
	if start != nil {
		countQuery = countQuery.Where("sent_at >= ?", *start)
	}
	if end != nil {
		countQuery = countQuery.Where("sent_at <= ?", *end)
	}
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("direction = ?", "inbound").
		Count(&inbound)
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("direction = ?", "outbound").
		Count(&outbound)
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("(is_read = ? OR is_read IS NULL)", false).
		Count(&unread)

	stats := &HubStatsResult{
		Total: total, Inbound: inbound, Outbound: outbound, Unread: unread,
		ByPlatform: map[string]int64{}, ByDirection: map[string]int64{},
		ByMsgType: map[string]int64{}, ByAccount: map[string]int64{},
	}

	type pcount struct {
		Platform string
		C        int64
	}
	var pCounts []pcount
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("1 = 1").
		Select("platform AS platform, COUNT(*) AS c").
		Group("platform").Scan(&pCounts)
	for _, p := range pCounts {
		stats.ByPlatform[p.Platform] = p.C
		stats.ByDirection["inbound_or_outbound"] += p.C
	}

	type dcount struct {
		Direction string
		C         int64
	}
	var dCounts []dcount
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("1 = 1").
		Select("direction AS direction, COUNT(*) AS c").
		Group("direction").Scan(&dCounts)
	for _, d := range dCounts {
		stats.ByDirection[d.Direction] = d.C
	}

	type tcount struct {
		MsgType string
		C       int64
	}
	var tCounts []tcount
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("1 = 1").
		Select("msg_type AS msg_type, COUNT(*) AS c").
		Group("msg_type").Scan(&tCounts)
	for _, t := range tCounts {
		stats.ByMsgType[t.MsgType] = t.C
	}

	type acount struct {
		AccountID string
		C         int64
	}
	var aCounts []acount
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("1 = 1").
		Select("account_id AS account_id, COUNT(*) AS c").
		Group("account_id").Order("c DESC").Limit(50).Scan(&aCounts)
	for _, a := range aCounts {
		stats.ByAccount[a.AccountID] = a.C
	}

	threshold24h := time.Now().Add(-24 * time.Hour)
	r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("sent_at >= ?", threshold24h).
		Count(&stats.Recent24h)

	return stats, nil
}

