package cauth

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/tusharsoni/copper/clogger"

	"github.com/tusharsoni/copper/chttp"
)

type router struct {
	req            chttp.BodyReader
	resp           chttp.Responder
	users          UsersSvc
	authMiddleware AuthMiddleware
	config         Config
	logger         clogger.Logger
}

func newRouter(
	req chttp.BodyReader,
	resp chttp.Responder,
	users UsersSvc,
	authMiddleware AuthMiddleware,
	config Config,
	logger clogger.Logger,
) *router {
	return &router{
		req:            req,
		resp:           resp,
		users:          users,
		authMiddleware: authMiddleware,
		config:         config,
		logger:         logger,
	}
}

func newChangePasswordRoute(ro *router) chttp.RouteResult {
	route := chttp.Route{
		Path:    "/api/user/change-password",
		Methods: []string{http.MethodPost},
		Handler: http.HandlerFunc(ro.changePassword),
	}
	return chttp.RouteResult{Route: route}
}

func (ro *router) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email" valid:"email"`
		OldPassword string `json:"old_password" valid:"printableascii"`
		NewPassword string `json:"new_password" valid:"printableascii"`
	}

	if !ro.req.Read(w, r, &body) {
		return
	}

	err := ro.users.ChangePassword(r.Context(), body.Email, body.OldPassword, body.NewPassword)
	if err != nil {
		ro.logger.Error("Failed to change password", err)
		ro.resp.InternalErr(w)
		return
	}

	ro.resp.OK(w, nil)
}

func newResetPasswordRoute(ro *router) chttp.RouteResult {
	route := chttp.Route{
		Path:    "/api/user/reset-password",
		Methods: []string{http.MethodPost},
		Handler: http.HandlerFunc(ro.resetPassword),
	}
	return chttp.RouteResult{Route: route}
}

func (ro *router) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email" valid:"email"`
	}

	if !ro.req.Read(w, r, &body) {
		return
	}

	err := ro.users.ResetPassword(r.Context(), body.Email)
	if err != nil {
		ro.logger.Error("Failed to reset password", err)
		ro.resp.InternalErr(w)
		return
	}

	ro.resp.OK(w, nil)
}

func newResendVerificationCodeRoute(ro *router) chttp.RouteResult {
	route := chttp.Route{
		MiddlewareFuncs: []chttp.MiddlewareFunc{ro.authMiddleware.AllowUnverified},
		Path:            "/api/user/resend-verification-code",
		Methods:         []string{http.MethodPost},
		Handler:         http.HandlerFunc(ro.resendVerificationCode),
	}
	return chttp.RouteResult{Route: route}
}

func (ro *router) resendVerificationCode(w http.ResponseWriter, r *http.Request) {
	user := GetCurrentUser(r.Context())

	err := ro.users.ResendVerificationCode(r.Context(), user.UUID)
	if err != nil {
		ro.logger.Error("Failed to resend verification code", err)
		ro.resp.InternalErr(w)
		return
	}

	ro.resp.OK(w, nil)
}

func newVerifyUserRoute(ro *router) chttp.RouteResult {
	route := chttp.Route{
		MiddlewareFuncs: []chttp.MiddlewareFunc{ro.authMiddleware.AllowUnverified},
		Path:            "/api/user/verify",
		Methods:         []string{http.MethodPost},
		Handler:         http.HandlerFunc(ro.verifyUser),
	}
	return chttp.RouteResult{Route: route}
}

func (ro *router) verifyUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		VerificationCode string `json:"verification_code" valid:"printableascii"`
	}

	if !ro.req.Read(w, r, &body) {
		return
	}

	err := ro.users.VerifyUser(r.Context(), GetCurrentUser(r.Context()).UUID, body.VerificationCode)
	if err != nil && err != ErrInvalidCredentials {
		ro.logger.Error("Failed to verify user", err)
		ro.resp.InternalErr(w)
		return
	} else if err == ErrInvalidCredentials {
		ro.resp.BadRequest(w, err)
		return
	}

	ro.resp.OK(w, nil)
}

func newLogoutRoute(ro *router, auth AuthMiddleware) chttp.RouteResult {
	route := chttp.Route{
		Path:            "/api/logout",
		MiddlewareFuncs: []chttp.MiddlewareFunc{auth.AllowUnverified},
		Methods:         []string{http.MethodPost},
		Handler:         http.HandlerFunc(ro.logout),
	}
	return chttp.RouteResult{Route: route}
}

func (ro *router) logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := GetCurrentUser(ctx)

	err := ro.users.Logout(ctx, user.UUID)
	if err != nil {
		ro.logger.Error("Failed to logout user", err)
		ro.resp.InternalErr(w)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:    "Authorization",
		Value:   "",
		Path:    "/",
		Expires: time.Unix(0, 0),
	})
}

func newLoginRoute(ro *router) chttp.RouteResult {
	route := chttp.Route{
		Path:    "/api/login",
		Methods: []string{http.MethodPost},
		Handler: http.HandlerFunc(ro.login),
	}
	return chttp.RouteResult{Route: route}
}

func (ro *router) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email" valid:"email"`
		Password string `json:"password" valid:"runelength(4|32)"`
	}

	if !ro.req.Read(w, r, &body) {
		return
	}

	u, sessionToken, err := ro.users.Login(r.Context(), body.Email, body.Password)
	if err != nil && err != ErrInvalidCredentials {
		ro.logger.Error("Failed to login user with email and password", err)
		ro.resp.InternalErr(w)
		return
	} else if err == ErrInvalidCredentials {
		ro.resp.Unauthorized(w)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:  "Authorization",
		Value: base64.StdEncoding.EncodeToString([]byte(u.Email + ":" + sessionToken)),
		Path:  "/",
	})

	ro.resp.OK(w, session{
		User:         u,
		SessionToken: sessionToken,
	})
}

func newSignupRoute(ro *router) chttp.RouteResult {
	route := chttp.Route{
		Path:    "/api/signup",
		Methods: []string{http.MethodPost},
		Handler: http.HandlerFunc(ro.signup),
	}
	return chttp.RouteResult{Route: route}
}

func (ro *router) signup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email" valid:"email"`
		Password string `json:"password" valid:"runelength(4|32)"`
	}

	if !ro.req.Read(w, r, &body) {
		return
	}

	user, sessionToken, err := ro.users.Signup(r.Context(), body.Email, body.Password)
	if err != nil && err != ErrUserAlreadyExists {
		ro.logger.Error("Failed to signup user with email and password", err)
		ro.resp.InternalErr(w)
		return
	} else if err == ErrUserAlreadyExists {
		ro.resp.BadRequest(w, ErrUserAlreadyExists)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:  "Authorization",
		Value: base64.StdEncoding.EncodeToString([]byte(user.Email + ":" + sessionToken)),
		Path:  "/",
	})

	ro.resp.Created(w, session{
		User:         user,
		SessionToken: sessionToken,
	})
}
