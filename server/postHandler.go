package server

import (
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/techagentng/citizenx/models"
	jwtPackage "github.com/techagentng/citizenx/services/jwt"
)

func (s *Server) handleCreatePost() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Handle file upload
        file, fileHeader, err := c.Request.FormFile("postImage")
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Missing or invalid file"})
            return
        }

        // Validate file type and size (same as for profile images)
        if err := validateFile(fileHeader); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }

        // Get the access token from the authorization header
        accessToken := getTokenFromHeader(c)
        if accessToken == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
            return
        }

        // Validate and decode the access token to get the userID
        secret := s.Config.JWTSecret
        accessClaims, err := jwtPackage.ValidateAndGetClaims(accessToken, secret)
        if err != nil {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
            return
        }

        var userID uint
        switch userIDValue := accessClaims["id"].(type) {
        case float64:
            userID = uint(userIDValue)
        default:
            c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid userID format"})
            return
        }

        // Validate form fields
        title := c.PostForm("title")
        postCategory := c.PostForm("post_category")
        postDescription := c.PostForm("post_description")

        if title == "" || postCategory == "" || postDescription == "" {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Title, category, and description are required"})
            return
        }

        // Create S3 client
        s3Client, err := createS3Client()
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create S3 client"})
            return
        }

        userIDString := strconv.FormatUint(uint64(userID), 10)

        // Generate unique filename
        filename := userIDString + "_" + fileHeader.Filename

        // Upload file to S3
        filepath, err := uploadFileToS3(s3Client, file, os.Getenv("AWS_BUCKET"), filename)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload file to S3"})
            return
        }

        // Create a new post
        post := models.Post{
            UserID:          userID,
            Title:           title,
            PostCategory:    postCategory,
            Image:           filepath,
            PostDescription: postDescription,
        }

        // Save the post to the database
        if err := s.PostRepository.CreatePost(&post); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create post"})
            return
        }

        c.JSON(http.StatusOK, gin.H{
            "message": "Post created successfully",
            "post":    post,
        })
    }
}
