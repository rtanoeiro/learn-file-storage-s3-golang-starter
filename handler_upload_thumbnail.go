package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", nil)
		return
	}

	fileExtension := strings.Split(mediaType, "/")[1]

	videoMetadata, videoErr := cfg.db.GetVideo(videoID)

	if videoErr != nil {
		respondWithError(w, http.StatusBadRequest, "Video does not exisst", videoErr)
		return
	}

	if userID != videoMetadata.UserID {
		respondWithError(w, http.StatusUnauthorized, "Unable to read file", nil)
		return
	}
	randomFileName, _ := CreateRandomFileName()
	filePath := filepath.Join(cfg.assetsRoot, fmt.Sprintf("%s.%s", randomFileName, fileExtension))
	log.Println("filePath", filePath)

	fileCreated, createError := os.Create(filePath)

	if createError != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create file in disk", createError)
		return
	}
	_, errorCopy := io.Copy(fileCreated, file)

	if errorCopy != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to copy file", errorCopy)
		return
	}

	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s.%s", cfg.port, randomFileName, fileExtension)

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
