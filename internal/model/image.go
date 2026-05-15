package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/oravandres/Logos/internal/database/dbq"
)

// ImageSource discriminates the origin of an Image row.
//
// Today the API maps three sources onto a single `images.url` column:
//   - SourceExternalURL: user-supplied http(s) URL; bytes live elsewhere.
//   - SourceUploaded:    user uploaded the file via multipart; bytes live in the local blobstore.
//   - SourceGenerated:   Logos asked an external generator (DarkBase image-adapter
//     today, Sparky later) for an image and persisted the bytes in the local
//     blobstore.
//
// The string values match the Postgres CHECK constraint on `images.source`
// in migration 000008.
const (
	SourceExternalURL = "external_url"
	SourceUploaded    = "uploaded"
	SourceGenerated   = "generated"
)

// ImageResponse is the JSON-serialisable representation of an image.
//
// Source-specific audit columns (`prompt`, `model`, `seed`, `generated_at`)
// are NULL for sources where they don't apply, and are emitted as JSON null
// — this keeps a single shape on the wire for every list/get response.
type ImageResponse struct {
	ID          uuid.UUID  `json:"id"`
	URL         string     `json:"url"`
	AltText     *string    `json:"alt_text"`
	CategoryID  *uuid.UUID `json:"category_id"`
	Source      string     `json:"source"`
	ContentType *string    `json:"content_type"`
	SizeBytes   *int64     `json:"size_bytes"`
	Width       *int32     `json:"width"`
	Height      *int32     `json:"height"`
	Prompt      *string    `json:"prompt"`
	Model       *string    `json:"model"`
	Seed        *int64     `json:"seed"`
	GeneratedAt *time.Time `json:"generated_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CreateImageRequest is the expected JSON body for creating an external-URL image.
//
// Multipart upload requests use a separate code path (handler.ImageHandler.Upload)
// rather than this struct so the binary stream never touches the JSON decoder.
type CreateImageRequest struct {
	URL        string     `json:"url"`
	AltText    *string    `json:"alt_text"`
	CategoryID *uuid.UUID `json:"category_id"`
}

// UpdateImageRequest is the expected JSON body for updating an image.
//
// Source-specific audit columns are immutable post-ingest by design; only
// the user-editable surface (URL, alt text, category) is accepted here.
type UpdateImageRequest struct {
	URL        string     `json:"url"`
	AltText    *string    `json:"alt_text"`
	CategoryID *uuid.UUID `json:"category_id"`
}

// imageFields gathers the union of columns every "image row" sqlc-generated
// type returns; centralising the lift keeps the per-row helpers below to a
// one-line call.
type imageFields struct {
	ID          [16]byte
	URL         string
	AltText     pgText
	CategoryID  [16]byte
	CategoryV   bool
	Source      string
	ContentType pgText
	SizeBytes   pgInt8
	Width       pgInt4
	Height      pgInt4
	Prompt      pgText
	Model       pgText
	Seed        pgInt8
	GeneratedAt pgTimestamptz
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type pgText struct {
	String string
	Valid  bool
}

type pgInt4 struct {
	Int32 int32
	Valid bool
}

type pgInt8 struct {
	Int64 int64
	Valid bool
}

type pgTimestamptz struct {
	Time  time.Time
	Valid bool
}

// imageResponseFromFields composes ImageResponse from the column union above.
func imageResponseFromFields(f imageFields) ImageResponse {
	resp := ImageResponse{
		ID:        uuid.UUID(f.ID),
		URL:       f.URL,
		Source:    f.Source,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
	if f.AltText.Valid {
		s := f.AltText.String
		resp.AltText = &s
	}
	if f.CategoryV {
		id := uuid.UUID(f.CategoryID)
		resp.CategoryID = &id
	}
	if f.ContentType.Valid {
		s := f.ContentType.String
		resp.ContentType = &s
	}
	if f.SizeBytes.Valid {
		n := f.SizeBytes.Int64
		resp.SizeBytes = &n
	}
	if f.Width.Valid {
		n := f.Width.Int32
		resp.Width = &n
	}
	if f.Height.Valid {
		n := f.Height.Int32
		resp.Height = &n
	}
	if f.Prompt.Valid {
		s := f.Prompt.String
		resp.Prompt = &s
	}
	if f.Model.Valid {
		s := f.Model.String
		resp.Model = &s
	}
	if f.Seed.Valid {
		n := f.Seed.Int64
		resp.Seed = &n
	}
	if f.GeneratedAt.Valid {
		t := f.GeneratedAt.Time
		resp.GeneratedAt = &t
	}
	return resp
}

// ImageFromCreateRow converts a sqlc CreateImageRow to the wire response.
func ImageFromCreateRow(img dbq.CreateImageRow) ImageResponse {
	return imageResponseFromFields(imageFields{
		ID: img.ID.Bytes, URL: img.Url,
		AltText:     pgText{img.AltText.String, img.AltText.Valid},
		CategoryID:  img.CategoryID.Bytes,
		CategoryV:   img.CategoryID.Valid,
		Source:      img.Source,
		ContentType: pgText{img.ContentType.String, img.ContentType.Valid},
		SizeBytes:   pgInt8{img.SizeBytes.Int64, img.SizeBytes.Valid},
		Width:       pgInt4{img.Width.Int32, img.Width.Valid},
		Height:      pgInt4{img.Height.Int32, img.Height.Valid},
		Prompt:      pgText{img.Prompt.String, img.Prompt.Valid},
		Model:       pgText{img.Model.String, img.Model.Valid},
		Seed:        pgInt8{img.Seed.Int64, img.Seed.Valid},
		GeneratedAt: pgTimestamptz{img.GeneratedAt.Time, img.GeneratedAt.Valid},
		CreatedAt:   img.CreatedAt.Time,
		UpdatedAt:   img.UpdatedAt.Time,
	})
}

// ImageFromUpdateRow converts a sqlc UpdateImageRow to the wire response.
func ImageFromUpdateRow(img dbq.UpdateImageRow) ImageResponse {
	return imageResponseFromFields(imageFields{
		ID: img.ID.Bytes, URL: img.Url,
		AltText:     pgText{img.AltText.String, img.AltText.Valid},
		CategoryID:  img.CategoryID.Bytes,
		CategoryV:   img.CategoryID.Valid,
		Source:      img.Source,
		ContentType: pgText{img.ContentType.String, img.ContentType.Valid},
		SizeBytes:   pgInt8{img.SizeBytes.Int64, img.SizeBytes.Valid},
		Width:       pgInt4{img.Width.Int32, img.Width.Valid},
		Height:      pgInt4{img.Height.Int32, img.Height.Valid},
		Prompt:      pgText{img.Prompt.String, img.Prompt.Valid},
		Model:       pgText{img.Model.String, img.Model.Valid},
		Seed:        pgInt8{img.Seed.Int64, img.Seed.Valid},
		GeneratedAt: pgTimestamptz{img.GeneratedAt.Time, img.GeneratedAt.Valid},
		CreatedAt:   img.CreatedAt.Time,
		UpdatedAt:   img.UpdatedAt.Time,
	})
}

// ImageFromGetRow converts a sqlc GetImageRow to the wire response.
func ImageFromGetRow(img dbq.GetImageRow) ImageResponse {
	return imageResponseFromFields(imageFields{
		ID: img.ID.Bytes, URL: img.Url,
		AltText:     pgText{img.AltText.String, img.AltText.Valid},
		CategoryID:  img.CategoryID.Bytes,
		CategoryV:   img.CategoryID.Valid,
		Source:      img.Source,
		ContentType: pgText{img.ContentType.String, img.ContentType.Valid},
		SizeBytes:   pgInt8{img.SizeBytes.Int64, img.SizeBytes.Valid},
		Width:       pgInt4{img.Width.Int32, img.Width.Valid},
		Height:      pgInt4{img.Height.Int32, img.Height.Valid},
		Prompt:      pgText{img.Prompt.String, img.Prompt.Valid},
		Model:       pgText{img.Model.String, img.Model.Valid},
		Seed:        pgInt8{img.Seed.Int64, img.Seed.Valid},
		GeneratedAt: pgTimestamptz{img.GeneratedAt.Time, img.GeneratedAt.Valid},
		CreatedAt:   img.CreatedAt.Time,
		UpdatedAt:   img.UpdatedAt.Time,
	})
}

// ImageFromUploadRow converts a sqlc CreateUploadedImageRow to the wire response.
func ImageFromUploadRow(img dbq.CreateUploadedImageRow) ImageResponse {
	return imageResponseFromFields(imageFields{
		ID: img.ID.Bytes, URL: img.Url,
		AltText:     pgText{img.AltText.String, img.AltText.Valid},
		CategoryID:  img.CategoryID.Bytes,
		CategoryV:   img.CategoryID.Valid,
		Source:      img.Source,
		ContentType: pgText{img.ContentType.String, img.ContentType.Valid},
		SizeBytes:   pgInt8{img.SizeBytes.Int64, img.SizeBytes.Valid},
		Width:       pgInt4{img.Width.Int32, img.Width.Valid},
		Height:      pgInt4{img.Height.Int32, img.Height.Valid},
		Prompt:      pgText{img.Prompt.String, img.Prompt.Valid},
		Model:       pgText{img.Model.String, img.Model.Valid},
		Seed:        pgInt8{img.Seed.Int64, img.Seed.Valid},
		GeneratedAt: pgTimestamptz{img.GeneratedAt.Time, img.GeneratedAt.Valid},
		CreatedAt:   img.CreatedAt.Time,
		UpdatedAt:   img.UpdatedAt.Time,
	})
}

// ImageFromGenerateRow converts a sqlc CreateGeneratedImageRow to the wire response.
func ImageFromGenerateRow(img dbq.CreateGeneratedImageRow) ImageResponse {
	return imageResponseFromFields(imageFields{
		ID: img.ID.Bytes, URL: img.Url,
		AltText:     pgText{img.AltText.String, img.AltText.Valid},
		CategoryID:  img.CategoryID.Bytes,
		CategoryV:   img.CategoryID.Valid,
		Source:      img.Source,
		ContentType: pgText{img.ContentType.String, img.ContentType.Valid},
		SizeBytes:   pgInt8{img.SizeBytes.Int64, img.SizeBytes.Valid},
		Width:       pgInt4{img.Width.Int32, img.Width.Valid},
		Height:      pgInt4{img.Height.Int32, img.Height.Valid},
		Prompt:      pgText{img.Prompt.String, img.Prompt.Valid},
		Model:       pgText{img.Model.String, img.Model.Valid},
		Seed:        pgInt8{img.Seed.Int64, img.Seed.Valid},
		GeneratedAt: pgTimestamptz{img.GeneratedAt.Time, img.GeneratedAt.Valid},
		CreatedAt:   img.CreatedAt.Time,
		UpdatedAt:   img.UpdatedAt.Time,
	})
}

// ImagesFromListRows converts a slice of sqlc ListImagesRow values to wire responses.
func ImagesFromListRows(imgs []dbq.ListImagesRow) []ImageResponse {
	out := make([]ImageResponse, len(imgs))
	for i, img := range imgs {
		out[i] = imageResponseFromFields(imageFields{
			ID: img.ID.Bytes, URL: img.Url,
			AltText:     pgText{img.AltText.String, img.AltText.Valid},
			CategoryID:  img.CategoryID.Bytes,
			CategoryV:   img.CategoryID.Valid,
			Source:      img.Source,
			ContentType: pgText{img.ContentType.String, img.ContentType.Valid},
			SizeBytes:   pgInt8{img.SizeBytes.Int64, img.SizeBytes.Valid},
			Width:       pgInt4{img.Width.Int32, img.Width.Valid},
			Height:      pgInt4{img.Height.Int32, img.Height.Valid},
			Prompt:      pgText{img.Prompt.String, img.Prompt.Valid},
			Model:       pgText{img.Model.String, img.Model.Valid},
			Seed:        pgInt8{img.Seed.Int64, img.Seed.Valid},
			GeneratedAt: pgTimestamptz{img.GeneratedAt.Time, img.GeneratedAt.Valid},
			CreatedAt:   img.CreatedAt.Time,
			UpdatedAt:   img.UpdatedAt.Time,
		})
	}
	return out
}
