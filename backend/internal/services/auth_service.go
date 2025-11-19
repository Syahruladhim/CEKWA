package services

import (
	"errors"
	"os"
	"time"

	"back_wa/internal/database"
	"back_wa/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct{}

type JWTClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Register creates a new user account
func (as *AuthService) Register(req models.UserRegister) (*models.UserResponse, error) {
	db := database.GetDB()

	// Check if email already exists
	var existingUser models.User
	if err := db.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		return nil, errors.New("email already registered")
	}

	// Check if username already exists
	if err := db.Where("username = ?", req.Username).First(&existingUser).Error; err == nil {
		return nil, errors.New("username already taken")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Create user
	user := models.User{
		Username:      req.Username,
		Email:         req.Email,
		PasswordHash:  string(hashedPassword),
		PhoneNumber:   req.PhoneNumber,
		Role:          "user",
		IsActive:      true,
		EmailVerified: true, // Set email as verified since OTP was already verified
	}

	if err := db.Create(&user).Error; err != nil {
		return nil, err
	}

	// Return user response (without password)
	return &models.UserResponse{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		PhoneNumber: user.PhoneNumber,
		Role:        user.Role,
		CreatedAt:   user.CreatedAt,
	}, nil
}

// Login authenticates user and returns JWT token
func (as *AuthService) Login(req models.UserLogin) (string, *models.UserResponse, error) {
	db := database.GetDB()

	// Find user by email
	var user models.User
	if err := db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		return "", nil, errors.New("invalid email or password")
	}

	// Check if user is active
	if !user.IsActive {
		return "", nil, errors.New("account is deactivated")
	}

	// Require verified email before allowing login
	if !user.EmailVerified {
		return "", nil, errors.New("email not verified. Please verify via OTP sent to your email")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return "", nil, errors.New("invalid email or password")
	}

	// Generate JWT token
	token, err := as.generateJWT(user)
	if err != nil {
		return "", nil, err
	}

	// Return token and user response
	userResponse := &models.UserResponse{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		PhoneNumber: user.PhoneNumber,
		Role:        user.Role,
		CreatedAt:   user.CreatedAt,
	}

	return token, userResponse, nil
}

// UpdatePassword updates a user's password hash
func (as *AuthService) UpdatePassword(user *models.User, newPassword string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	db := database.GetDB()
	user.PasswordHash = string(hashedPassword)
	return db.Save(user).Error
}

// generateJWT creates a JWT token for the user
func (as *AuthService) generateJWT(user models.User) (string, error) {
	// JWT secret key from environment variable
	secretKey := os.Getenv("JWT_SECRET")
	if secretKey == "" {
		secretKey = "wa-analyzer-super-secret-jwt-key-2024-change-in-production" // fallback
	}

	claims := JWTClaims{
		UserID:   user.ID,
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 24 hours
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

// ValidateToken validates JWT token and returns user claims
func (as *AuthService) ValidateToken(tokenString string) (*JWTClaims, error) {
	secretKey := os.Getenv("JWT_SECRET")
	if secretKey == "" {
		secretKey = "wa-analyzer-super-secret-jwt-key-2024-change-in-production" // fallback
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secretKey), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// GetUserByID retrieves user by ID
func (as *AuthService) GetUserByID(userID uint) (*models.UserResponse, error) {
	db := database.GetDB()

	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		return nil, err
	}

	return &models.UserResponse{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		PhoneNumber: user.PhoneNumber,
		Role:        user.Role,
		CreatedAt:   user.CreatedAt,
	}, nil
}
