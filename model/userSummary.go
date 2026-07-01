package model

type UserSummary struct {
	Id       uint   `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

func (s *UserSummary) FromUser(user *User) *UserSummary {
	if user == nil || user.Id == 0 {
		return nil
	}
	s.Id = user.Id
	s.Username = user.Username
	s.Nickname = user.Nickname
	return s
}
