package db

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/techagentng/citizenx/models"
	"gorm.io/gorm"
)

type AuthRepository interface {
	CreateUser(user *models.User) (*models.User, error)
	CreateGoogleUser(user *models.CreateSocialUserParams) (*models.CreateSocialUserParams, error)
	IsEmailExist(email string) error
	IsPhoneExist(email string) error
	FindUserByUsername(username string) (*models.User, error)
	FindUserByEmail(email string) (*models.User, error)
	UpdateUser(user *models.User) error
	AddToBlackList(blacklist *models.Blacklist) error
	TokenInBlacklist(token string) bool
	VerifyEmail(email string, token string) error
	IsTokenInBlacklist(token string) bool
	UpdatePassword(password string, email string) error
	FindUserByID(id uint) (*models.User, error)
	// UpdateUserImage(user *models.User) error
	EditUserProfile(userID uint, userDetails *models.EditProfileResponse) error
	FindUserByMacAddress(macAddress string) (*models.LoginRequestMacAddress, error)
	ResetPassword(userID, NewPassword string) error
	CreateUserWithMacAddress(user *models.LoginRequestMacAddress) (*models.LoginRequestMacAddress, error)
	UpdateUserStatus(user *models.User) error
	UpdateUserOnlineStatus(user *models.User) error
	SetUserOffline(user *models.User) error
	GetOnlineUserCount() (int64, error)
	GetAllUsers() ([]models.User, error)
	CreateUserImage(user *models.User) error
}

type authRepo struct {
	DB *gorm.DB
}

func NewAuthRepo(db *GormDB) AuthRepository {
	return &authRepo{db.DB}
}

func (a *authRepo) CreateUser(user *models.User) (*models.User, error) {
	if user == nil {
		log.Println("CreateUser error: user is nil")
		return nil, errors.New("user is nil")
	}

	// Create the user in the database
	err := a.DB.Create(user).Error
	if err != nil {
		log.Printf("CreateUser error: %v", err)
		return nil, err
	}

	return user, nil
}

// CreateUserWithMacAddress updates the MAC address field for an existing user or creates a new user with the provided MAC address
func (a *authRepo) CreateUserWithMacAddress(user *models.LoginRequestMacAddress) (*models.LoginRequestMacAddress, error) {
	// Attempt to find an existing user with the same MAC address
	existingUser := &models.User{}
	err := a.DB.Where("mac_address = ?", user.MacAddress).First(existingUser).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Println("DB error:", err)
		return nil, fmt.Errorf("could not find existing user: %v", err)
	}

	// If no existing user is found, create a new user with the provided MAC address
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = a.DB.Create(user).Error
		if err != nil {
			log.Println("DB error:", err)
			return nil, fmt.Errorf("could not create user: %v", err)
		}
	}

	return user, nil
}

func (a *authRepo) CreateGoogleUser(user *models.CreateSocialUserParams) (*models.CreateSocialUserParams, error) {
	err := a.DB.Create(user).Error
	if err != nil {
		return nil, fmt.Errorf("could not create user: %v", err)
	}
	return user, nil
}

func (a *authRepo) FindUserByUsername(username string) (*models.User, error) {
	db := a.DB
	user := &models.User{}
	err := db.Where("email = ? OR username = ?", username, username).First(user).Error
	if err != nil {
		return nil, fmt.Errorf("could not find user: %v", err)
	}
	return user, nil
}

func (a *authRepo) IsEmailExist(email string) error {
	var count int64
	err := a.DB.Model(&models.User{}).Where("email = ?", email).Count(&count).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No user found with this email, return nil
			return nil
		}
		// Return wrapped error for other errors
		return errors.Wrap(err, "gorm count error")
	}
	if count > 0 {
		// Email already exists, return specific error
		return errors.New("email already in use")
	}
	return nil
}

func (a *authRepo) IsPhoneExist(phone string) error {
	var count int64
	err := a.DB.Model(&models.User{}).Where("telephone = ?", phone).Count(&count).Error
	if err != nil {
		return errors.Wrap(err, "gorm.count error")
	}
	if count > 0 {
		return fmt.Errorf("phone number already in use")
	}
	return nil
}

func (a *authRepo) FindUserByEmail(email string) (*models.User, error) {
	var user models.User
	err := a.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("error finding user by email: %w", err)
	}
	return &user, nil
}


func (a *authRepo) UpdateUser(user *models.User) error {
	return nil
}

func (a *authRepo) AddToBlackList(blacklist *models.Blacklist) error {
	result := a.DB.Create(blacklist)
	return result.Error
}

func (a *authRepo) TokenInBlacklist(token string) bool {
	result := a.DB.Where("token = ?", token).Find(&models.Blacklist{})
	return result.Error != nil
}

func (a *authRepo) VerifyEmail(email string, token string) error {
	err := a.DB.Model(&models.User{}).Where("email = ?", email).Updates(models.User{IsEmailActive: true}).Error
	if err != nil {
		return err
	}

	err = a.AddToBlackList(&models.Blacklist{Token: token})
	return err
}

func normalizeToken(token string) string {
	// Trim leading and trailing white spaces
	return strings.TrimSpace(token)
}

func (a *authRepo) IsTokenInBlacklist(token string) bool {
	// Normalize the token
	normalizedToken := normalizeToken(token)

	var count int64
	// Assuming you have a Blacklist model with a Token field
	a.DB.Model(&models.Blacklist{}).Where("token = ?", normalizedToken).Count(&count)
	return count > 0
}

func (a *authRepo) UpdatePassword(password string, email string) error {
	err := a.DB.Model(&models.User{}).Where("email = ?", email).Updates(models.User{HashedPassword: password}).Error
	if err != nil {
		return err
	}
	return nil
}

func (a *authRepo) FindUserByID(id uint) (*models.User, error) {
	var user models.User
	err := a.DB.Where("id = ?", id).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
}

func (a *authRepo) FindUserByMacAddress(macAddress string) (*models.LoginRequestMacAddress, error) {
	var user models.LoginRequestMacAddress
	err := a.DB.Where("mac_address = ?", macAddress).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
}

func (a *authRepo) CreateUserImage(user *models.User) error {
    // Assuming you have a UserImage model or similar
    newUserImage := models.UserImage{
        UserID:      user.ID,
        ThumbNailURL: user.ThumbNailURL,
        CreatedAt:   time.Now(),
    }

    result := a.DB.Create(&newUserImage)
    if result.Error != nil {
        log.Printf("Error creating user image in database: %v", result.Error)
        return result.Error
    }

    if result.RowsAffected == 0 {
        log.Println("No rows affected when creating user image")
        return errors.New("failed to create user image")
    }

    return nil
}


func (a *authRepo) EditUserProfile(userID uint, userDetails *models.EditProfileResponse) error {
	// Fetch the user from the database
	user := models.User{}
	if err := a.DB.First(&user, userID).Error; err != nil {
		return err // return error if user not found
	}

	// Update user details based on userDetails
	user.Fullname = userDetails.Fullname
	user.Username = userDetails.Username
	// Update other fields as needed

	// Perform the update operation
	if err := a.DB.Save(&user).Error; err != nil {
		return err
	}

	return nil
}

func (a *authRepo) ResetPassword(userID, NewPassword string) error {
	result := a.DB.Model(models.User{}).Where("id = ?", userID).Update("hashed_password", NewPassword)
	return result.Error
}

// Function in your repository to update the user's status
func (a *authRepo) UpdateUserStatus(user *models.User) error {
	return a.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("online", user.Online).Error
}

func (a *authRepo) UpdateUserOnlineStatus(user *models.User) error {
	log.Printf("Attempting to update user status: ID=%d, Online=%v", user.ID, user.Online)
	result := a.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("online", user.Online)
	if result.Error != nil {
		log.Printf("Error updating user status: %v", result.Error)
		return result.Error
	}
	if result.RowsAffected == 0 {
		log.Printf("No rows affected when updating user status for user ID: %d", user.ID)
		return fmt.Errorf("no rows affected")
	}
	log.Printf("Successfully updated user status for user ID: %d", user.ID)
	return nil
}

func (a *authRepo) SetUserOffline(user *models.User) error {
	log.Printf("Attempting to set user status to offline: ID=%d", user.ID)
	result := a.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("online", false)
	if result.Error != nil {
		log.Printf("Error setting user status to offline: %v", result.Error)
		return result.Error
	}
	if result.RowsAffected == 0 {
		log.Printf("No rows affected when setting user status to offline for user ID: %d", user.ID)
		return fmt.Errorf("no rows affected")
	}
	log.Printf("Successfully set user status to offline for user ID: %d", user.ID)
	return nil
}

func (a *authRepo) GetOnlineUserCount() (int64, error) {
	var count int64
	result := a.DB.Model(&models.User{}).Where("online = ?", true).Count(&count)
	if result.Error != nil {
		log.Printf("Error fetching online user count: %v", result.Error)
		return 0, result.Error
	}
	return count, nil
}

func (a *authRepo) GetAllUsers() ([]models.User, error) {
	var users []models.User
	result := a.DB.Find(&users)
	if result.Error != nil {
		log.Printf("Error fetching all users: %v", result.Error)
		return nil, result.Error
	}
	return users, nil
}
