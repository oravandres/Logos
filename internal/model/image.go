package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/oravandres/Logos/internal/database/dbq"
)

// ImageResponse is the JSON-serialisable representation of an image.
type ImageResponse struct {
	ID         uuid.UUID  `json:"id"`
	URL        string     `json:"url"`
	AltText    *string    `json:"alt_text"`
	CategoryID *uuid.UUID `json:"category_id"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateImageRequest is the expected JSON body for creating an image.
type CreateImageRequest struct {
	URL        string     `json:"url"`
	AltText    *string    `json:"alt_text"`
	CategoryID *uuid.UUID `json:"category_id"`
}

// UpdateImageRequest is the expected JSON body for updating an image.
type UpdateImageRequest struct {
	URL        string     `json:"url"`
	AltText    *string    `json:"alt_text"`
	CategoryID *uuid.UUID `json:"category_id"`
}

// ImageFromDB converts a database row into an ImageResponse.
func ImageFromDB(img dbq.Image) ImageResponse {
	resp := ImageResponse{
		ID:        uuidFromPgtype(img.ID),
		URL:       img.Url,
		CreatedAt: img.CreatedAt.Time,
		UpdatedAt: img.UpdatedAt.Time,
	}
	if img.AltText.Valid {
		resp.AltText = &img.AltText.String
	}
	if img.CategoryID.Valid {
		id := uuidFromPgtype(img.CategoryID)
		resp.CategoryID = &id
	}
	return resp
}

// ImagesFromDB converts a slice of database rows into ImageResponse values.
func ImagesFromDB(imgs []dbq.Image) []ImageResponse {
	out := make([]ImageResponse, len(imgs))
	for i, img := range imgs {
		out[i] = ImageFromDB(img)
	}
	return out
}
