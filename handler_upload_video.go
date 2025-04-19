package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/utils"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

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

	videoMetadata, dbError := cfg.db.GetVideo(videoID)
	if dbError != nil {
		respondWithError(w, http.StatusNotFound, "Video not found", dbError)
		return
	}

	uploadedFile, header, errForm := r.FormFile("video")
	if errForm != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get form data from HTTP request", errForm)
	}
	defer uploadedFile.Close()

	contentType := header.Header.Get("Content-Type")
	mediaType, _, errMedia := mime.ParseMediaType(contentType)
	fileExtension := strings.Split(mediaType, "/")[1]

	if errMedia != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to parse media type", errMedia)
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", nil)
		return
	}

	tempFile, errFile := os.CreateTemp("/tmp", "tubely-*.mp4")
	if errFile != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create temp file", errFile)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, errCopy := io.Copy(tempFile, uploadedFile)
	if errCopy != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to copy uploaded file to temp file", errCopy)
		return
	}

	fastStartFile, fastErrr := utils.ProcessVideoForFastStart(tempFile.Name())
	if fastErrr != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to process video for fast start", fastErrr)
		return
	}

	fastStartFileBytes, errOpen := os.Open(fastStartFile)
	if errOpen != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to open processed video file", errOpen)
		return
	}
	defer os.Remove(fastStartFile)
	defer fastStartFileBytes.Close()

	_, errOffset := uploadedFile.Seek(0, io.SeekStart)
	if errOffset != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to set offset for file", errOffset)
		return
	}

	ratio, errStream := utils.GetVideoAspectRatio(fastStartFile)
	if errStream != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video aspect ratio", errStream)
		return
	}

	keyFileName, _ := CreateRandomFileName()
	videoFullURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s/%s.%s", cfg.s3Bucket, cfg.s3Region, ratio, keyFileName, fileExtension)
	keyURL := fmt.Sprintf("%s/%s.%s", ratio, keyFileName, fileExtension)

	objectInput := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &keyURL,
		Body:        fastStartFileBytes,
		ContentType: &mediaType,
	}
	_, errUpload := cfg.s3AppClient.PutObject(r.Context(), &objectInput)
	log.Println("Uploading video to S3", videoFullURL)
	if errUpload != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to upload file to S3", errUpload)
	}
	updateVideoParams := database.Video{
		CreateVideoParams: database.CreateVideoParams{
			Title:       videoMetadata.Title,
			Description: videoMetadata.Description,
			UserID:      videoMetadata.UserID,
		},
		ThumbnailURL: videoMetadata.ThumbnailURL,
		ID:           videoID,
		VideoURL:     &videoFullURL,
	}
	updateError := cfg.db.UpdateVideo(updateVideoParams)
	if updateError != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video in database", updateError)
	}

}

func CreateRandomFileName() (string, error) {
	randomBytes := make([]byte, 32)
	_, errBytes := rand.Read(randomBytes)
	if errBytes != nil {
		return "", errors.New("unable to create file name")
	}
	randomFileName := base64.RawURLEncoding.EncodeToString(randomBytes)
	return randomFileName, nil
}
