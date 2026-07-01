package service

import "github.com/lejianwen/rustdesk-api/v2/model"

func userSummaryMap(userIDs []uint) map[uint]*model.UserSummary {
	result := make(map[uint]*model.UserSummary)
	if len(userIDs) == 0 {
		return result
	}
	seen := make(map[uint]struct{}, len(userIDs))
	ids := make([]uint, 0, len(userIDs))
	for _, id := range userIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return result
	}
	var users []*model.User
	DB.Where("id in ?", ids).Find(&users)
	for _, user := range users {
		if user == nil || user.Id == 0 {
			continue
		}
		result[user.Id] = (&model.UserSummary{}).FromUser(user)
	}
	return result
}

func attachLoginLogUserSummaries(logs []*model.LoginLog) {
	ids := make([]uint, 0, len(logs))
	for _, log := range logs {
		if log != nil {
			ids = append(ids, log.UserId)
		}
	}
	summaries := userSummaryMap(ids)
	for _, log := range logs {
		if log != nil {
			log.User = summaries[log.UserId]
		}
	}
}

func attachUserTokenUserSummaries(tokens []model.UserToken) {
	ids := make([]uint, 0, len(tokens))
	for _, token := range tokens {
		ids = append(ids, token.UserId)
	}
	summaries := userSummaryMap(ids)
	for i := range tokens {
		tokens[i].User = summaries[tokens[i].UserId]
	}
}
