package repository

import (
	"github.com/tigerowo/infinite-canvas/model"
	"gorm.io/gorm/clause"
)

// SaveCasdoorLogin creates one identity once; profile refresh never overwrites balance, role or password.
func SaveCasdoorLogin(input model.User, create bool) (model.User, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, err
	}
	if create {
		if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&input).Error; err != nil {
			return model.User{}, err
		}
	}
	values := map[string]any{"display_name": input.DisplayName, "last_login_at": input.CreatedAt, "updated_at": input.CreatedAt}
	if input.Email != "" {
		values["email"] = input.Email
	}
	if err := db.Model(&model.User{}).Where("id = ? AND casdoor_id = ? AND status = ?", input.ID, input.CasdoorID, model.UserStatusActive).Updates(values).Error; err != nil {
		return model.User{}, err
	}
	var user model.User
	err = db.Where("id = ? AND casdoor_id = ?", input.ID, input.CasdoorID).First(&user).Error
	return user, err
}
