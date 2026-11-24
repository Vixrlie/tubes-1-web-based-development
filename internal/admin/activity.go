package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
)

type activityOutcomeKey struct{}

func markActivityFailed(r *http.Request) {
	if failed, ok := r.Context().Value(activityOutcomeKey{}).(*bool); ok {
		*failed = true
	}
}

var adminActions = map[string]string{
	"POST /admin/users":                               "admin.user.create",
	"POST /admin/users/{id}":                          "admin.user.update",
	"POST /admin/users/{id}/activate":                 "admin.user.activate",
	"POST /admin/users/{id}/deactivate":               "admin.user.deactivate",
	"POST /admin/users/{id}/password":                 "admin.user.reset_password",
	"POST /admin/users/{id}/delete":                   "admin.user.delete",
	"POST /admin/groups":                              "admin.group.create",
	"POST /admin/groups/{id}/delete":                  "admin.group.delete",
	"POST /admin/groups/{id}/members":                 "admin.group.member.add",
	"POST /admin/groups/{id}/members/{userID}/delete": "admin.group.member.remove",
	"POST /admin/groups/{id}/access":                  "admin.group.access.update",
	"POST /admin/connections":                         "admin.connection.create",
	"POST /admin/connections/{id}":                    "admin.connection.update",
	"POST /admin/connections/{id}/delete":             "admin.connection.delete",
	"POST /admin/connections/test":                    "admin.connection.test",
	"POST /admin/connections/{id}/test":               "admin.connection.test",
}

func AdminActivityMiddleware(logger *activity.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if logger == nil {
				next.ServeHTTP(w, r)
				return
			}

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			var failed bool
			r = r.WithContext(context.WithValue(r.Context(), activityOutcomeKey{}, &failed))
			next.ServeHTTP(ww, r)

			routeCtx := chi.RouteContext(r.Context())
			if routeCtx == nil {
				return
			}
			pattern := routeCtx.RoutePattern()
			action, ok := adminActions[r.Method+" "+pattern]
			if !ok {
				return
			}

			actor := "unknown"
			if user, found := auth.UserFromContext(r.Context()); found {
				actor = strconv.FormatInt(user.ID, 10)
			}

			statusCode := ww.Status()
			if statusCode == 0 {
				statusCode = http.StatusOK
			}
			result := activity.StatusSuccess
			var eventErr error
			if failed || statusCode >= http.StatusBadRequest {
				result = activity.StatusFailed
				eventErr = errors.New("admin request failed")
			}

			details := map[string]string{
				"method":      r.Method,
				"route":       pattern,
				"http_status": strconv.Itoa(statusCode),
			}
			if id := chi.URLParam(r, "id"); id != "" {
				if _, err := strconv.ParseInt(id, 10, 64); err == nil {
					details["resource_id"] = id
				}
			}
			if id := chi.URLParam(r, "userID"); id != "" {
				if _, err := strconv.ParseInt(id, 10, 64); err == nil {
					details["target_user_id"] = id
				}
			}

			logger.Log(r.Context(), activity.Event{
				UserID: actor,
				Action: action,
				Status: result,
				Detail: details,
				Err:    eventErr,
			})
		})
	}
}
