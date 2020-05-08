package service

import (
	"context"
	"log"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/influenzanet/user-management-service/pkg/api"
	"github.com/influenzanet/user-management-service/pkg/models"
	"github.com/influenzanet/user-management-service/pkg/pwhash"
	utils "github.com/influenzanet/user-management-service/utils"
)

func (s *userManagementServer) Status(ctx context.Context, _ *empty.Empty) (*api.ServiceStatus, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *userManagementServer) LoginWithEmail(ctx context.Context, req *api.LoginWithEmailMsg) (*api.UserAuthInfo, error) {
	if req == nil || req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid username and/or password")
	}

	instanceID := req.InstanceId
	if instanceID == "" {
		instanceID = "default"
	}
	user, err := s.userDBservice.GetUserByEmail(instanceID, req.Email)

	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid username and/or password")
	}

	match, err := pwhash.ComparePasswordWithHash(user.Account.Password, req.Password)
	if err != nil || !match {
		return nil, status.Error(codes.InvalidArgument, "invalid username and/or password")
	}

	if err := s.userDBservice.UpdateLoginTime(instanceID, user.ID.Hex()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	var username string
	if len(user.Roles) > 1 || len(user.Roles) == 1 && user.Roles[0] != "PARTICIPANT" {
		username = user.Account.AccountID
	}

	apiUser := user.ToAPI()

	response := &api.UserAuthInfo{
		UserId:            user.ID.Hex(),
		Roles:             user.Roles,
		InstanceId:        instanceID,
		AccountId:         username, // relevant for researchers
		AccountConfirmed:  apiUser.Account.AccountConfirmedAt > 0,
		Profiles:          apiUser.Profiles,
		SelectedProfile:   apiUser.Profiles[0],
		PreferredLanguage: apiUser.Account.PreferredLanguage,
	}
	return response, nil

}

func (s *userManagementServer) SignupWithEmail(ctx context.Context, req *api.SignupWithEmailMsg) (*api.UserAuthInfo, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "missing argument")
	}
	if !utils.CheckEmailFormat(req.Email) {
		return nil, status.Error(codes.InvalidArgument, "email not valid")
	}
	if !utils.CheckPasswordFormat(req.Password) {
		return nil, status.Error(codes.InvalidArgument, "password too weak")
	}

	password, err := pwhash.HashPassword(req.Password)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Create user DB object from request:
	newUser := models.User{
		Account: models.Account{
			Type:               "email",
			AccountID:          req.Email,
			AccountConfirmedAt: 0, // not confirmed yet
			Password:           password,
			PreferredLanguage:  req.PreferredLanguage,
		},
		Roles: []string{"PARTICIPANT"},
		Profiles: []models.Profile{
			{
				ID:                 primitive.NewObjectID(),
				Nickname:           req.Email,
				ConsentConfirmedAt: time.Now().Unix(),
				AvatarID:           "default",
			},
		},
	}
	newUser.AddNewEmail(req.Email, false)

	if req.WantsNewsletter {
		newUser.ContactPreferences.SubscribedToNewsletter = true
		newUser.ContactPreferences.SendNewsletterTo = []string{newUser.ContactInfos[0].ID.Hex()}
	}

	instanceID := req.InstanceId
	if instanceID == "" {
		instanceID = "default"
	}

	id, err := s.userDBservice.AddUser(instanceID, newUser)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.Println("TODO: generate account confirmation token for newly created user")
	log.Println("TODO: send email for newly created user")
	// TODO: generate email confirmation token
	// TODO: send email with confirmation request

	var username string
	if len(newUser.Roles) > 1 || len(newUser.Roles) == 1 && newUser.Roles[0] != "PARTICIPANT" {
		username = newUser.Account.AccountID
	}

	apiUser := newUser.ToAPI()

	response := &api.UserAuthInfo{
		UserId:            id,
		Roles:             newUser.Roles,
		InstanceId:        instanceID,
		AccountId:         username, // relevant for researchers
		AccountConfirmed:  false,
		Profiles:          apiUser.Profiles,
		SelectedProfile:   apiUser.Profiles[0],
		PreferredLanguage: apiUser.Account.PreferredLanguage,
	}
	return response, nil
}

func (s *userManagementServer) CheckRefreshToken(ctx context.Context, req *api.RefreshTokenRequest) (*api.ServiceStatus, error) {
	if req == nil || req.RefreshToken == "" || req.UserId == "" || req.InstanceId == "" {
		return nil, status.Error(codes.InvalidArgument, "missing argument")
	}
	user, err := s.userDBservice.GetUserByID(req.InstanceId, req.UserId)
	if err != nil {
		return nil, status.Error(codes.Internal, "user not found")
	}

	err = user.RemoveRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, status.Error(codes.Internal, "token not found")
	}
	user.Timestamps.LastTokenRefresh = time.Now().Unix()

	user, err = s.userDBservice.UpdateUser(req.InstanceId, user)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &api.ServiceStatus{
		Status: api.ServiceStatus_NORMAL,
		Msg:    "refresh token removed",
	}, nil
}

func (s *userManagementServer) TokenRefreshed(ctx context.Context, req *api.RefreshTokenRequest) (*api.ServiceStatus, error) {
	if req == nil || req.RefreshToken == "" || req.UserId == "" || req.InstanceId == "" {
		return nil, status.Error(codes.InvalidArgument, "missing argument")
	}

	user, err := s.userDBservice.GetUserByID(req.InstanceId, req.UserId)
	if err != nil {
		return nil, status.Error(codes.Internal, "user not found")
	}
	user.AddRefreshToken(req.RefreshToken)
	user.Timestamps.LastTokenRefresh = time.Now().Unix()

	user, err = s.userDBservice.UpdateUser(req.InstanceId, user)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &api.ServiceStatus{
		Status: api.ServiceStatus_NORMAL,
		Msg:    "token refresh time updated",
	}, nil
}

func (s *userManagementServer) SwitchProfile(ctx context.Context, req *api.ProfileRequest) (*api.UserAuthInfo, error) {
	if req == nil || utils.IsTokenEmpty(req.Token) || req.Profile == nil {
		return nil, status.Error(codes.InvalidArgument, "missing argument")
	}
	user, err := s.userDBservice.GetUserByID(req.Token.InstanceId, req.Token.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, "user not found")
	}

	profile, err := user.FindProfile(req.Profile.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, "profile not found")
	}

	apiUser := user.ToAPI()

	response := &api.UserAuthInfo{
		UserId:            user.ID.Hex(),
		Roles:             user.Roles,
		InstanceId:        req.Token.InstanceId,
		AccountConfirmed:  apiUser.Account.AccountConfirmedAt > 0,
		AccountId:         apiUser.Account.AccountId,
		Profiles:          apiUser.Profiles,
		SelectedProfile:   profile.ToAPI(),
		PreferredLanguage: apiUser.Account.PreferredLanguage,
	}
	return response, nil
}