package service

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

type TagService struct {
}

func (s *TagService) Info(id uint) *model.Tag {
	p := &model.Tag{}
	DB.Where("id = ?", id).First(p)
	return p
}
func (s *TagService) InfoByUserIdAndNameAndCollectionId(userid uint, name string, cid uint) *model.Tag {
	p := &model.Tag{}
	DB.Where("user_id = ? and name = ? and collection_id = ?", userid, name, cid).First(p)
	return p
}

func (s *TagService) ListByUserId(userId uint) (res *model.TagList) {
	res = s.List(1, 1000, func(tx *gorm.DB) {
		tx.Where("user_id = ?", userId)
	})
	return
}
func (s *TagService) ListByUserIdAndCollectionId(userId, cid uint) (res *model.TagList) {
	res = s.List(1, 1000, func(tx *gorm.DB) {
		tx.Where("user_id = ? and collection_id = ?", userId, cid)
		tx.Order("name asc")
	})
	return
}
func (s *TagService) UpdateTags(userId uint, tags map[string]uint) error {
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()
	//先查询所有tag
	var allTags []*model.Tag
	if err := tx.Where("user_id = ?", userId).Find(&allTags).Error; err != nil {
		tx.Rollback()
		return err
	}
	for _, t := range allTags {
		if _, ok := tags[t.Name]; !ok {
			//删除
			if err := tx.Delete(t).Error; err != nil {
				tx.Rollback()
				return err
			}
		} else {
			if tags[t.Name] != t.Color {
				//更新
				t.Color = tags[t.Name]
				if err := tx.Save(t).Error; err != nil {
					tx.Rollback()
					return err
				}
			}
			//移除
			delete(tags, t.Name)
		}
	}
	//新增
	for tag, color := range tags {
		t := &model.Tag{}
		t.Name = tag
		t.Color = color
		t.UserId = userId
		if err := tx.Create(t).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

// InfoById 根据用户id取用户信息
func (s *TagService) InfoById(id uint) *model.Tag {
	u := &model.Tag{}
	DB.Where("id = ?", id).First(u)
	return u
}

func (s *TagService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.TagList) {
	page, pageSize = NormalizePagination(page, pageSize)
	res = &model.TagList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.Tag{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Tags)
	return
}

// Create 创建
func (s *TagService) Create(u *model.Tag) error {
	res := DB.Create(u).Error
	return res
}
func (s *TagService) Delete(u *model.Tag) error {
	return DB.Delete(u).Error
}
func (s *TagService) BatchDelete(ids []uint) error {
	return DB.Where("id in ?", ids).Delete(&model.Tag{}).Error
}

// Update 更新
func (s *TagService) Update(u *model.Tag) error {
	return DB.Model(u).Select("*").Omit("created_at").Updates(u).Error
}
