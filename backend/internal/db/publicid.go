package db

import (
	"crypto/rand"
	"fmt"
	"strconv"

	"gorm.io/gorm"
)

const publicIDAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NewPublicID returns a 12-character random id for public URLs.
func NewPublicID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("p%x", randFallback())
	}
	out := make([]byte, 12)
	for i, b := range buf {
		out[i] = publicIDAlphabet[int(b)%len(publicIDAlphabet)]
	}
	return string(out)
}

func randFallback() []byte {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return b
}

func (p *Project) BeforeCreate(tx *gorm.DB) error {
	if p.PublicID == "" {
		p.PublicID = NewPublicID()
	}
	return nil
}

// FindProjectByRef accepts a public id or a legacy numeric id.
func FindProjectByRef(ref string) (*Project, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var project Project
	if ref == "" {
		return nil, gorm.ErrRecordNotFound
	}
	q := DB.Where("public_id = ?", ref)
	if _, err := strconv.ParseUint(ref, 10, 64); err == nil {
		q = DB.Where("public_id = ? OR id = ?", ref, ref)
	}
	if err := q.First(&project).Error; err != nil {
		return nil, err
	}
	if project.PublicID == "" {
		project.PublicID = NewPublicID()
		DB.Model(&project).Update("public_id", project.PublicID)
	}
	return &project, nil
}

// BackfillPublicIDs adds a nullable public_id, fills existing rows, then
// AutoMigrate can safely apply the unique index.
func BackfillPublicIDs(gdb *gorm.DB) {
	if !gdb.Migrator().HasTable(&Project{}) {
		return
	}
	if !gdb.Migrator().HasColumn(&Project{}, "public_id") {
		_ = gdb.Exec(`ALTER TABLE projects ADD COLUMN IF NOT EXISTS public_id varchar(16)`)
	}
	var ids []uint
	if err := gdb.Table("projects").Where("public_id IS NULL OR public_id = ''").Pluck("id", &ids).Error; err != nil {
		return
	}
	for _, id := range ids {
		_ = gdb.Table("projects").Where("id = ?", id).Update("public_id", NewPublicID())
	}
}
