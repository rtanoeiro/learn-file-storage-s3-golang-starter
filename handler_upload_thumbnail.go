package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)

	// "thumbnail" should match the HTML form input name
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")

	fileData, errFile := io.ReadAll(file)

	if errFile != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to read file", errFile)
	}

	videoMetadata, videoErr := cfg.db.GetVideo(videoID)

	if videoErr != nil {
		respondWithError(w, http.StatusBadRequest, "Video does not exisst", videoErr)
		return
	}

	if userID != videoMetadata.UserID {
		respondWithError(w, http.StatusUnauthorized, "Unable to read file", errFile)
		return
	}

	videoThumbnail := thumbnail{
		data:      fileData,
		mediaType: mediaType,
	}
	videoThumbnails[videoID] = videoThumbnail

	thumbnailURL := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", cfg.port, videoID.String())
	updateParams := database.Video{
		ID:           videoMetadata.ID,
		CreatedAt:    videoMetadata.CreatedAt,
		UpdatedAt:    videoMetadata.UpdatedAt,
		ThumbnailURL: &thumbnailURL,
		VideoURL:     videoMetadata.VideoURL,
		CreateVideoParams: database.CreateVideoParams{
			Title:       videoMetadata.Title,
			Description: videoMetadata.Description,
			UserID:      userID,
		},
	}
	errUpdate := cfg.db.UpdateVideo(updateParams)
	if errUpdate != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to Update Video", errUpdate)
		return
	}

	newVideoMetadata, newvideoErr := cfg.db.GetVideo(videoID)
	if newvideoErr != nil {
		respondWithError(w, http.StatusBadRequest, "Video does not exisst", videoErr)
		return
	}
	respondWithJSON(w, http.StatusOK, newVideoMetadata)
}
