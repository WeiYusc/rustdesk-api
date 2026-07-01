package model

// Setting stores operator-editable admin panel settings as module-scoped JSON.
type Setting struct {
	IdModel
	Key       string `json:"key" gorm:"uniqueIndex;not null;size:128"`
	Value     string `json:"value" gorm:"type:text;not null"`
	IsSecret  bool   `json:"is_secret" gorm:"default:0;not null"`
	UpdatedBy uint   `json:"updated_by" gorm:"default:0;not null;index"`
	TimeModel
}
