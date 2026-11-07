package dao

import (
	"database/sql"
	"errors"
	"fmt"

	"hackathon-backend/model"
	"hackathon-backend/usecase"
)

type ImageDao struct {
	DB *sql.DB
}

var _ usecase.ImageStore = (*ImageDao)(nil)

func (d *ImageDao) Insert(img model.Image) error {
	_, err := d.DB.Exec("INSERT INTO images (id, owner_id, content_type, data, created_at) VALUES (?, ?, ?, ?, ?)",
		img.Id, img.OwnerId, img.ContentType, img.Data, img.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert image: %w", err)
	}
	return nil
}

func (d *ImageDao) Get(id string) (model.Image, error) {
	var img model.Image
	err := d.DB.QueryRow("SELECT id, owner_id, content_type, data, created_at FROM images WHERE id = ?", id).
		Scan(&img.Id, &img.OwnerId, &img.ContentType, &img.Data, &img.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Image{}, model.ErrNotFound
	}
	if err != nil {
		return model.Image{}, fmt.Errorf("select image: %w", err)
	}
	return img, nil
}

func (d *ImageDao) OwnerOf(id string) (string, error) {
	var owner string
	err := d.DB.QueryRow("SELECT owner_id FROM images WHERE id = ?", id).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return "", model.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("select image owner: %w", err)
	}
	return owner, nil
}
