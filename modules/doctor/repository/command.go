package repository

import (
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"talkspace-api/modules/doctor/entity"
	"talkspace-api/modules/doctor/model"
	"talkspace-api/utils/bcrypt"
	"talkspace-api/utils/constant"
	"talkspace-api/utils/helper/cloud"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type doctorCommandRepository struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewDoctorCommandRepository(db *gorm.DB, rdb *redis.Client) DoctorCommandRepositoryInterface {
	return &doctorCommandRepository{
		db:  db,
		rdb: rdb,
	}
}

func (dcr *doctorCommandRepository) RegisterDoctor(doctor entity.Doctor, image *multipart.FileHeader) (entity.Doctor, error) {
	doctorModel := entity.DoctorEntityToDoctorModel(doctor)

	result := dcr.db.Create(&doctorModel)
	if result.Error != nil {
		return entity.Doctor{}, result.Error
	}

	doctorEntity := entity.DoctorModelToDoctorEntity(doctorModel)

	if image != nil {
        imageURL, errUpload := cloud.UploadImageToS3(image)
        if errUpload != nil {
            return entity.Doctor{}, errUpload
        }
        doctorModel.ProfilePicture = imageURL
    }

	data, err := json.Marshal(doctorEntity)
	if err != nil {
		return entity.Doctor{}, err
	}

	cacheKey := "doctor:" + doctorEntity.ID
	err = dcr.rdb.Set(context.Background(), cacheKey, data, 24*time.Hour).Err()
	if err != nil {
		return entity.Doctor{}, err
	}

	return doctorEntity, nil
}

func (dcr *doctorCommandRepository) LoginDoctor(email, password string) (entity.Doctor, error) {
	cacheKey := "doctor:" + email

	cachedData, err := dcr.rdb.Get(context.Background(), cacheKey).Result()
	if err == redis.Nil {
		doctorModel := model.Doctor{}

		result := dcr.db.Where("email = ?", email).First(&doctorModel)
		if result.Error != nil {
			return entity.Doctor{}, result.Error
		}

		if errComparePass := bcrypt.ComparePassword(doctorModel.Password, password); errComparePass != nil {
			return entity.Doctor{}, errors.New(constant.ERROR_PASSWORD_INVALID)
		}

		doctorEntity := entity.DoctorModelToDoctorEntity(doctorModel)

		data, _ := json.Marshal(doctorEntity)
		err = dcr.rdb.Set(context.Background(), cacheKey, data, 24*time.Hour).Err()
		if err != nil {
			return entity.Doctor{}, err
		}

		return doctorEntity, nil
	} else if err != nil {
		return entity.Doctor{}, err
	}

	doctorEntity := entity.Doctor{}
	if err := json.Unmarshal([]byte(cachedData), &doctorEntity); err != nil {
		return entity.Doctor{}, err
	}

	return doctorEntity, nil
}

func (dcr *doctorCommandRepository) UpdateDoctorProfile(id string, doctor entity.Doctor, image *multipart.FileHeader) (entity.Doctor, error) {
	doctorModel := entity.DoctorEntityToDoctorModel(doctor)

	if doctorModel.ID == "" {
		doctorModel.ID = id
	}

	if image != nil {
        imageURL, errUpload := cloud.UploadImageToS3(image)
        if errUpload != nil {
            return entity.Doctor{}, errUpload
        }
        doctorModel.ProfilePicture = imageURL
    }

	result := dcr.db.Where("id = ?", id).Updates(&doctorModel)
	if result.Error != nil {
		return entity.Doctor{}, result.Error
	}

	if result.RowsAffected == 0 {
		return entity.Doctor{}, errors.New(constant.ERROR_ID_NOTFOUND)
	}

	doctorEntity := entity.DoctorModelToDoctorEntity(doctorModel)
	data, err := json.Marshal(doctorEntity)
	if err != nil {
		return entity.Doctor{}, err
	}

	cacheKey := "doctor:" + id
	err = dcr.rdb.Set(context.Background(), cacheKey, data, 24*time.Hour).Err()
	if err != nil {
		return entity.Doctor{}, err
	}

	return doctorEntity, nil
}

func (dcr *doctorCommandRepository) UpdateDoctorStatus(id string, status bool) (entity.Doctor, error) {
	doctorModel := model.Doctor{}
	result := dcr.db.Where("id = ?", id).First(&doctorModel)
	if result.Error != nil {
		return entity.Doctor{}, result.Error
	}

	if result.RowsAffected == 0 {
		return entity.Doctor{}, errors.New(constant.ERROR_ID_NOTFOUND)
	}

	doctorModel.Status = status
	result = dcr.db.Save(&doctorModel)
	if result.Error != nil {
		return entity.Doctor{}, result.Error
	}

	doctorEntity := entity.DoctorModelToDoctorEntity(doctorModel)

	data, err := json.Marshal(doctorEntity)
	if err != nil {
		return entity.Doctor{}, err
	}

	cacheKey := "doctor:" + id
	err = dcr.rdb.Set(context.Background(), cacheKey, data, 24*time.Hour).Err()
	if err != nil {
		return entity.Doctor{}, err
	}

	return doctorEntity, nil
}

func (dcr *doctorCommandRepository) SendDoctorOTP(email string, otp string, expired int64) (entity.Doctor, error) {
	doctorModel := model.Doctor{}

	result := dcr.db.Where("email = ?", email).First(&doctorModel)
	if result.Error != nil {
		if result.RowsAffected == 0 {
			return entity.Doctor{}, errors.New(constant.ERROR_EMAIL_NOTFOUND)
		}
		return entity.Doctor{}, result.Error
	}

	cacheKey := "otp:" + email
	err := dcr.rdb.Set(context.Background(), cacheKey, otp, time.Duration(expired)*time.Second).Err()
	if err != nil {
		return entity.Doctor{}, err
	}

	doctorModel.OTP = otp
	doctorModel.OTPExpiration = expired

	errUpdate := dcr.db.Save(&doctorModel).Error
	if errUpdate != nil {
		return entity.Doctor{}, errUpdate
	}

	doctorEntity := entity.DoctorModelToDoctorEntity(doctorModel)

	return doctorEntity, nil
}

func (dcr *doctorCommandRepository) VerifyDoctorOTP(email, otp string) (entity.Doctor, error) {
	cacheKey := "otp:" + email

	cachedOTP, err := dcr.rdb.Get(context.Background(), cacheKey).Result()
	if err == redis.Nil || cachedOTP != otp {
		return entity.Doctor{}, errors.New(constant.ERROR_EMAIL_OTP)
	} else if err != nil {
		return entity.Doctor{}, err
	}

	doctorModel := model.Doctor{}
	result := dcr.db.Where("otp = ? AND email = ?", otp, email).First(&doctorModel)
	if result.Error != nil {
		return entity.Doctor{}, result.Error
	}

	doctorEntity := entity.DoctorModelToDoctorEntity(doctorModel)

	dcr.rdb.Del(context.Background(), cacheKey)

	return doctorEntity, nil
}

func (dcr *doctorCommandRepository) ResetDoctorOTP(otp string) (entity.Doctor, error) {
	doctorModel := model.Doctor{}

	result := dcr.db.Where("otp = ?", otp).First(&doctorModel)
	if result.Error != nil {
		return entity.Doctor{}, result.Error
	}

	if result.RowsAffected == 0 {
		return entity.Doctor{}, errors.New(constant.ERROR_OTP_NOTFOUND)
	}

	doctorModel.OTP = ""
	doctorModel.OTPExpiration = 0

	errUpdate := dcr.db.Save(&doctorModel).Error
	if errUpdate != nil {
		return entity.Doctor{}, errUpdate
	}

	doctorEntity := entity.DoctorModelToDoctorEntity(doctorModel)

	return doctorEntity, nil
}

func (dcr *doctorCommandRepository) UpdateDoctorPassword(id string, password entity.Doctor) (entity.Doctor, error) {

	doctorModel := entity.DoctorEntityToDoctorModel(password)

	result := dcr.db.Where("id = ?", id).Updates(&doctorModel)
	if result.Error != nil {
		return entity.Doctor{}, result.Error
	}

	if result.RowsAffected == 0 {
		return entity.Doctor{}, errors.New(constant.ERROR_ID_NOTFOUND)
	}

	doctorEntity := entity.DoctorModelToDoctorEntity(doctorModel)

	return doctorEntity, nil
}

func (dcr *doctorCommandRepository) NewDoctorPassword(email string, password entity.Doctor) (entity.Doctor, error) {
	doctorModel := model.Doctor{}

	result := dcr.db.Where("email = ?", email).First(&doctorModel)
	if result.Error != nil {
		return entity.Doctor{}, result.Error
	}

	if result.RowsAffected == 0 {
		return entity.Doctor{}, errors.New(constant.ERROR_EMAIL_NOTFOUND)
	}

	errUpdate := dcr.db.Model(&doctorModel).Updates(entity.DoctorEntityToDoctorModel(password))
	if errUpdate != nil {
		return entity.Doctor{}, errUpdate.Error
	}

	doctorEntity := entity.DoctorModelToDoctorEntity(doctorModel)
	data, err := json.Marshal(doctorEntity)
	if err != nil {
		return entity.Doctor{}, err
	}

	cacheKey := "doctor:" + doctorEntity.ID
	err = dcr.rdb.Set(context.Background(), cacheKey, data, 24*time.Hour).Err()
	if err != nil {
		return entity.Doctor{}, err
	}

	return doctorEntity, nil
}
