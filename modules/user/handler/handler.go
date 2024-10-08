package handler

import (
	"net/http"
	"strings"
	"talkspace-api/middlewares"
	"talkspace-api/modules/user/dto"
	"talkspace-api/modules/user/usecase"
	"talkspace-api/utils/constant"
	"talkspace-api/utils/helper/cloud"
	"talkspace-api/utils/responses"

	"github.com/labstack/echo/v4"
)

type userHandler struct {
	userCommandUsecase usecase.UserCommandUsecaseInterface
	userQueryUsecase   usecase.UserQueryUsecaseInterface
}

func NewUserHandler(ucu usecase.UserCommandUsecaseInterface, uqu usecase.UserQueryUsecaseInterface) *userHandler {
	return &userHandler{
		userCommandUsecase: ucu,
		userQueryUsecase:   uqu,
	}
}

// Query
func (uh *userHandler) GetUserByID(c echo.Context) error {
	userIDParam := c.Param("user_id")
	if userIDParam == "" {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(constant.ERROR_ID_NOTFOUND))
	}

	tokenUserID, role, errExtract := middlewares.ExtractToken(c)
	if errExtract != nil {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(errExtract.Error()))
	}

	if role != constant.USER {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(constant.ERROR_ROLE_ACCESS))
	}

	if userIDParam != tokenUserID {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(constant.ERROR_ROLE_ACCESS))
	}

	user, errGetID := uh.userQueryUsecase.GetUserByID(userIDParam)
	if errGetID != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errGetID.Error()))
	}

	userResponse := dto.UserEntityToUserProfileResponse(user)

	return c.JSON(http.StatusOK, responses.SuccessResponse(constant.SUCCESS_PROFILE_RETRIEVED, userResponse))
}

// Command
func (uh *userHandler) RegisterUser(c echo.Context) error {
	userRequest := dto.UserRegisterRequest{}

	errBind := c.Bind(&userRequest)
	if errBind != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errBind.Error()))
	}

	userEntity := dto.UserRegisterRequestToUserEntity(userRequest)

	registeredUser, errRegister := uh.userCommandUsecase.RegisterUser(userEntity)
	if errRegister != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errRegister.Error()))
	}

	userResponse := dto.UserEntityToUserRegisterResponse(registeredUser)

	return c.JSON(http.StatusCreated, responses.SuccessResponse(constant.SUCCESS_REGISTER, userResponse))
}

func (uh *userHandler) LoginUser(c echo.Context) error {
	userRequest := dto.UserLoginRequest{}

	errBind := c.Bind(&userRequest)
	if errBind != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errBind.Error()))
	}

	LoginUser, token, errLogin := uh.userCommandUsecase.LoginUser(userRequest.Email, userRequest.Password)
	if errLogin != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errLogin.Error()))
	}

	userResponse := dto.UserEntityToUserLoginResponse(LoginUser, token)

	return c.JSON(http.StatusOK, responses.SuccessResponse(constant.SUCCESS_LOGIN, userResponse))
}

func (uh *userHandler) UpdateUserProfile(c echo.Context) error {
	userIDParam := c.Param("user_id")
	if userIDParam == "" {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(constant.ERROR_ID_NOTFOUND))
	}

	tokenUserID, role, errExtract := middlewares.ExtractToken(c)
	if errExtract != nil {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(errExtract.Error()))
	}

	if role != constant.USER {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(constant.ERROR_ROLE_ACCESS))
	}

	if userIDParam != tokenUserID {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(constant.ERROR_ROLE_ACCESS))
	}

	userRequest := dto.UserUpdateProfileRequest{}

	errBind := c.Bind(&userRequest)
	if errBind != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errBind.Error()))
	}

	image, errFile := c.FormFile("profile_picture")
	if errFile != nil && errFile != http.ErrMissingFile {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(constant.ERROR_UPLOAD_IMAGE))
	}

	if image != nil {
		imageURL, errUpload := cloud.UploadImageToS3(image)
		if errUpload != nil {
			return c.JSON(http.StatusInternalServerError, responses.ErrorResponse(constant.ERROR_UPLOAD_IMAGE_S3))
		}
		userRequest.ProfilePicture = imageURL
	}

	userEntity := dto.UserUpdateProfileRequestToUserEntity(userRequest)

	user, errUpdate := uh.userCommandUsecase.UpdateUserProfile(userIDParam, userEntity, image)
	if errUpdate != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errUpdate.Error()))
	}

	userResponse := dto.UserEntityToUserUpdateProfileResponse(user)

	return c.JSON(http.StatusOK, responses.SuccessResponse(constant.SUCCESS_PROFILE_UPDATED, userResponse))
}

func (uh *userHandler) UpdateUserPassword(c echo.Context) error {
	userRequest := dto.UserUpdatePasswordRequest{}

	errBind := c.Bind(&userRequest)
	if errBind != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errBind.Error()))
	}

	userID, role, errExtractToken := middlewares.ExtractToken(c)

	if role != constant.USER {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(constant.ERROR_ROLE_ACCESS))
	}

	if errExtractToken != nil {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(errExtractToken.Error()))
	}

	userEntity := dto.UserUpdatePasswordRequestToUserEntity(userRequest)

	_, errUpdate := uh.userCommandUsecase.UpdateUserPassword(userID, userEntity)
	if errUpdate != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errUpdate.Error()))
	}

	return c.JSON(http.StatusOK, responses.SuccessResponse(constant.SUCCESS_PASSWORD_UPDATED, nil))
}

func (uh *userHandler) ForgotUserPassword(c echo.Context) error {
	userRequest := dto.UserSendOTPRequest{}

	errBind := c.Bind(&userRequest)
	if errBind != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errBind.Error()))
	}

	userEntity := dto.UserSendOTPRequestToUserEntity(userRequest)

	otp, errSendOTP := uh.userCommandUsecase.SendUserOTP(userEntity.Email)
	if errSendOTP != nil {
		if strings.Contains(errSendOTP.Error(), constant.ERROR_EMAIL_NOTFOUND) {
			return c.JSON(http.StatusNotFound, responses.ErrorResponse(errSendOTP.Error()))
		}
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errSendOTP.Error()))
	}

	userResponse := dto.UserEntityToUserResponse(otp)

	return c.JSON(http.StatusOK, responses.SuccessResponse(constant.SUCCESS_OTP_SENT, userResponse))
}

func (uh *userHandler) VerifyUserOTP(c echo.Context) error {
	userRequest := dto.UserVerifyOTPRequest{}

	errBind := c.Bind(&userRequest)
	if errBind != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errBind.Error()))
	}

	userEntity := dto.UserVerifyOTPRequestToUserEntity(userRequest)

	token, errVerify := uh.userCommandUsecase.VerifyUserOTP(userEntity.Email, userEntity.OTP)
	if errVerify != nil {
		return c.JSON(http.StatusInternalServerError, responses.ErrorResponse(constant.ERROR_OTP_VERIFY+errVerify.Error()))
	}

	return c.JSON(http.StatusOK, responses.SuccessResponse(constant.SUCCESS_OTP_VERIFIED, token))
}

func (uh *userHandler) NewUserPassword(c echo.Context) error {
	userRequest := dto.UserNewPasswordRequest{}

	errBind := c.Bind(&userRequest)
	if errBind != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errBind.Error()))
	}

	email, errExtract := middlewares.ExtractVerifyToken(c)
	if errExtract != nil {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(errExtract.Error()))
	}

	userEntity := dto.UserNewPasswordRequestToUserEntity(userRequest)

	_, errCreate := uh.userCommandUsecase.NewUserPassword(email, userEntity)
	if errCreate != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errCreate.Error()))
	}

	return c.JSON(http.StatusOK, responses.SuccessResponse(constant.SUCCESS_PASSWORD_UPDATED, nil))
}

func (uh *userHandler) RequestPremium(c echo.Context) error {
	request_premium := c.Param("request_premium")

	userID, role, errExtractToken := middlewares.ExtractToken(c)
	if errExtractToken != nil {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(errExtractToken.Error()))
	}

	if role != constant.USER {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(constant.ERROR_ROLE_ACCESS))
	}

	userEntity, errGet := uh.userQueryUsecase.GetUserByID(userID)
	if errGet != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errGet.Error()))
	}

	_, errCreate := uh.userCommandUsecase.RequestPremium(userEntity, request_premium)
	if errCreate != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errCreate.Error()))
	}

	return c.JSON(http.StatusCreated, responses.SuccessResponse(constant.SUCCESS_REQUEST_PREMIUM, nil))
}

func (uh *userHandler) UpdateUserPremiumExpired(c echo.Context) error {
	userVerify := dto.UserVerifyPremium{}

	errBind := c.Bind(&userVerify)
	if errBind != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errBind.Error()))
	}

	_, role, errExtractToken := middlewares.ExtractToken(c)
	if errExtractToken != nil {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(errExtractToken.Error()))
	}

	if role != "admin" {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(constant.ERROR_ROLE_ACCESS))
	}

	_, errUpdate := uh.userCommandUsecase.UpdateUserPremiumExpired(userVerify.UserID, userVerify.Status)
	if errUpdate != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errUpdate.Error()))
	}

	return c.JSON(http.StatusOK, responses.SuccessResponse(constant.SUCCESS_PREMIUM_EXPIRED, nil))
}

func (uh *userHandler) GetRequestPremiumUsers(c echo.Context) error {
	_, role, errExtractToken := middlewares.ExtractToken(c)
	if errExtractToken != nil {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(errExtractToken.Error()))
	}

	if role != "admin" {
		return c.JSON(http.StatusUnauthorized, responses.ErrorResponse(constant.ERROR_ROLE_ACCESS))
	}

	users, errGet := uh.userQueryUsecase.GetRequestPremiumUsers()
	if errGet != nil {
		return c.JSON(http.StatusBadRequest, responses.ErrorResponse(errGet.Error()))
	}

	usersResponse := dto.ListUserEntityToUserListResponse(users)

	return c.JSON(http.StatusOK, responses.SuccessResponse(constant.SUCCESS_REQUEST_PREMIUM, usersResponse))
}