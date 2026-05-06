package controller

import (
	"fmt"
	"time"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

const dashboardTopWorkgroupsLimit = 5

func dashboardWindowDuration(window api.DashboardWindow) (time.Duration, error) {
	switch window {
	case api.DashboardWindow24h:
		return 24 * time.Hour, nil
	case api.DashboardWindow7d:
		return 7 * 24 * time.Hour, nil
	case api.DashboardWindow30d:
		return 30 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("window must be 24h, 7d, or 30d")
	}
}

func dashboardBucketDuration(bucket api.DashboardBucket) (time.Duration, error) {
	switch bucket {
	case api.DashboardBucket1h:
		return time.Hour, nil
	case api.DashboardBucket6h:
		return 6 * time.Hour, nil
	case api.DashboardBucket1d:
		return 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("bucket must be 1h, 6h, or 1d")
	}
}

func mapDashboardStats(stats persistence.DashboardStats) api.DashboardStats {
	return api.DashboardStats{
		ActiveSessions:        stats.ActiveSessions,
		ActiveSessionsDelta7d: stats.ActiveSessionsDelta7d,
		EnvelopesToday:        stats.EnvelopesToday,
		EnvelopesYesterday:    stats.EnvelopesYesterday,
		ActiveWorkgroups:      stats.ActiveWorkgroups,
		ActiveTunnels:         stats.ActiveTunnels,
	}
}

func mapDashboardRibbon(ribbon persistence.DashboardRibbon) api.DashboardRibbon {
	return api.DashboardRibbon{
		WorkgroupCount:     ribbon.WorkgroupCount,
		AdvertisementCount: ribbon.AdvertisementCount,
		SessionsToday:      ribbon.SessionsToday,
		EnvironmentCount:   ribbon.EnvironmentCount,
	}
}

func mapDashboardBuckets(rows []persistence.Bucket) []api.DashboardActivityBucket {
	result := make([]api.DashboardActivityBucket, 0, len(rows))
	for _, row := range rows {
		result = append(result, api.DashboardActivityBucket{
			Start:     row.Start,
			Envelopes: row.Envelopes,
			Sessions:  row.Sessions,
		})
	}
	return result
}

func mapDashboardWorkgroupActivity(rows []persistence.WorkgroupEnvelopes) []api.DashboardWorkgroupActivity {
	result := make([]api.DashboardWorkgroupActivity, 0, len(rows))
	for _, row := range rows {
		name := row.WorkgroupName
		if name == "" {
			name = row.WorkgroupID
		}
		result = append(result, api.DashboardWorkgroupActivity{
			WorkgroupId:   row.WorkgroupID,
			WorkgroupName: name,
			Envelopes:     row.Envelopes,
		})
	}
	return result
}

func mapDashboardEnvironments(rows []persistence.EnvironmentStatus) api.DashboardEnvironmentsResponse {
	result := make(api.DashboardEnvironmentsResponse, 0, len(rows))
	for _, row := range rows {
		env := api.DashboardEnvironment{
			ID:        row.ID,
			Name:      row.Name,
			AccountId: row.AccountID,
			Status:    api.DashboardEnvironmentStatus(row.Status),
		}
		if row.LastHeartbeatAt != nil {
			env.LastHeartbeatAt.SetTo(*row.LastHeartbeatAt)
		}
		result = append(result, env)
	}
	return result
}
