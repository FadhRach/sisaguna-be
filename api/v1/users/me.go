package users

import "net/http"

// Me adalah entrypoint Vercel untuk GET dan PATCH /api/v1/users/me.
func Me(w http.ResponseWriter, r *http.Request) {
	requireAuth(container.UserHandler.Me)(w, r)
}
