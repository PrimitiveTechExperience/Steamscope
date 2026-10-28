package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/models"
	"github.com/redis/go-redis/v9"
)

// Account lockout: after lockoutThreshold failed sign-ins within
// lockoutWindow, an account takes no password attempts for lockoutDuration,
// from any address. This is on top of the per-address rate limit, which a
// spread-out guessing attack gets around.
//
// Failures are counted for login names that don't exist as well, the same
// way, so a lockout says nothing about whether an account exists. A real
// account is counted by its ID, so switching between username and email
// doesn't buy extra attempts. Signing in with Steam and resetting the
// password both still work while an account is locked.
const (
	lockoutThreshold = 10
	lockoutWindow    = time.Hour
	lockoutDuration  = 30 * time.Minute
)

func lockoutKey(user *models.User, login string) string {
	if user != nil {
		return userLockoutKey(user.UserID)
	}
	return "lockout:login:" + strings.ToLower(login)
}

func userLockoutKey(userID int64) string {
	return "lockout:user:" + strconv.FormatInt(userID, 10)
}

// lockedFor returns how much longer the key is locked, or 0. A Redis error
// counts as not locked, like the rate limiter.
func (h *Handler) lockedFor(ctx context.Context, key string) time.Duration {
	count, err := h.Redis.Get(ctx, key).Int64()
	if err != nil || count < lockoutThreshold {
		return 0
	}
	ttl, err := h.Redis.PTTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		return 0
	}
	return ttl
}

// failureScript counts a failed sign-in. The count lives for lockoutWindow
// from the first failure; reaching the threshold restarts the expiry at the
// lockout duration, which is then the time left on the lock.
//
// KEYS[1] the counter; ARGV[1] the window, ARGV[2] the lock, ARGV[3] the threshold.
var failureScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == tonumber(ARGV[3]) then
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
elseif count == 1 or redis.call('PTTL', KEYS[1]) < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`)

// recordLoginFailure counts a failure and reports whether it locked the account.
func (h *Handler) recordLoginFailure(ctx context.Context, key string) bool {
	count, err := failureScript.Run(ctx, h.Redis, []string{key},
		lockoutWindow.Milliseconds(), lockoutDuration.Milliseconds(), lockoutThreshold).Int64()
	if err != nil {
		log.Printf("lockout: could not count a failed sign-in: %v", err)
		return false
	}
	return count == lockoutThreshold
}

func (h *Handler) clearLockout(ctx context.Context, key string) {
	if err := h.Redis.Del(ctx, key).Err(); err != nil {
		log.Printf("lockout: could not clear %s: %v", key, err)
	}
}

// lockoutMessage is the same for every login name, real or not.
func lockoutMessage(left time.Duration) string {
	minutes := int(left.Round(time.Minute).Minutes())
	if minutes < 1 {
		minutes = 1
	}
	unit := "minutes"
	if minutes == 1 {
		unit = "minute"
	}
	return fmt.Sprintf("too many failed sign-in attempts for this account. Try again in %d %s, or reset your password", minutes, unit)
}

// sendLockoutEmail tells the owner their account was locked, so a guessing
// attack doesn't go unnoticed. It runs in the background.
func (h *Handler) sendLockoutEmail(user *models.User) {
	if h.Alerts == nil || h.Alerts.Mail == nil {
		return
	}
	body := fmt.Sprintf("Hi %s,\n\n"+
		"Someone entered the wrong password for your Steamscope account %d times, so signing in with a password "+
		"is paused for %d minutes.\n\n"+
		"If that was you, wait and try again. If it wasn't, nobody has got in, but consider choosing a new password: %s/forgot-password\n",
		user.Username, lockoutThreshold, int(lockoutDuration.Minutes()), h.Config.FrontendURL)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.Alerts.Mail.Send(ctx, user.Email, "Steamscope: sign-in paused after failed attempts", body); err != nil {
			log.Printf("lockout: email to user %d failed: %v", user.UserID, err)
		}
	}()
}
