package service

import (
	gen "github.com/gly-hub/quickgo/example/framework/auth-server/api/proto/gen"
	"github.com/gly-hub/quickgo/grpcep"
)

// newLoginResponse 创建并初始化 LoginResponse
func newLoginResponse() *gen.LoginResponse {
	resp := &gen.LoginResponse{}
	grpcep.InitResponse(&resp)
	return resp
}

// newVerifyTokenResponse 创建并初始化 VerifyTokenResponse
func newVerifyTokenResponse() *gen.VerifyTokenResponse {
	resp := &gen.VerifyTokenResponse{}
	grpcep.InitResponse(&resp)
	return resp
}

// newRefreshTokenResponse 创建并初始化 RefreshTokenResponse
func newRefreshTokenResponse() *gen.RefreshTokenResponse {
	resp := &gen.RefreshTokenResponse{}
	grpcep.InitResponse(&resp)
	return resp
}

// newGetUserInfoResponse 创建并初始化 GetUserInfoResponse
func newGetUserInfoResponse() *gen.GetUserInfoResponse {
	resp := &gen.GetUserInfoResponse{}
	grpcep.InitResponse(&resp)
	return resp
}
