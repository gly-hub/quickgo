package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// UserModel 用户模型（数据库表结构）
type UserModel struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	UserID    string         `gorm:"uniqueIndex;not null;size:64" json:"user_id"` // 用户ID
	Username  string         `gorm:"uniqueIndex;not null;size:64" json:"username"` // 用户名
	Password  string         `gorm:"not null;size:255" json:"-"`                   // 密码（不返回）
	Email     string         `gorm:"size:128" json:"email"`                         // 邮箱
	Nickname  string         `gorm:"size:64" json:"nickname"`                       // 昵称
	Avatar    string         `gorm:"size:255" json:"avatar"`                        // 头像
	Roles     string         `gorm:"size:255" json:"roles"`                         // 角色（逗号分隔）
	Status    int            `gorm:"default:1" json:"status"`                       // 状态：1-正常，0-禁用
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 指定表名
func (UserModel) TableName() string {
	return "users"
}

// GetRoles 获取角色列表
func (u *UserModel) GetRoles() []string {
	if u.Roles == "" {
		return []string{}
	}
	// 按逗号分割角色字符串
	parts := strings.Split(u.Roles, ",")
	roles := make([]string, 0, len(parts))
	for _, part := range parts {
		role := strings.TrimSpace(part)
		if role != "" {
			roles = append(roles, role)
		}
	}
	return roles
}

// SetRoles 设置角色列表
func (u *UserModel) SetRoles(roles []string) {
	if len(roles) == 0 {
		u.Roles = ""
		return
	}
	// 使用逗号连接角色
	u.Roles = strings.Join(roles, ",")
}

