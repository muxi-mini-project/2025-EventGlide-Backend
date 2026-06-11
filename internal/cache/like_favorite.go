package cache

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type Subject string

const (
	SubjectActivity Subject = "activity"
	SubjectPost     Subject = "post"
	SubjectComment  Subject = "comment"
)

// LikeSetKey 点赞 Set 的 Redis Key
func LikeSetKey(subject Subject, subjectID int64) string {
	return fmt.Sprintf("%s:like:%d", subject, subjectID)
}

// LikeCountKey 点赞计数的 Redis Key
func LikeCountKey(subject Subject, subjectID int64) string {
	return fmt.Sprintf("%s:like:count:%d", subject, subjectID)
}

// CollectSetKey 收藏 Set 的 Redis Key
func CollectSetKey(subject Subject, subjectID int64) string {
	return fmt.Sprintf("%s:collect:%d", subject, subjectID)
}

// CollectCountKey 收藏计数的 Redis Key
func CollectCountKey(subject Subject, subjectID int64) string {
	return fmt.Sprintf("%s:collect:count:%d", subject, subjectID)
}

// LikeFavoriteRedis 点赞收藏 Redis 操作封装
type LikeFavoriteRedis struct {
	rdb *redis.Client
}

// Lua 点赞
const likeScript = `
local added = redis.call("SADD", KEYS[1], ARGV[1])
if added == 1 then
    redis.call("INCR", KEYS[2])
end
return added
`

// Lua 取消点赞
const unlikeScript = `
local removed = redis.call("SREM", KEYS[1], ARGV[1])
if removed == 1 then
    local v = redis.call("DECR", KEYS[2])
    if v < 0 then
        redis.call("SET", KEYS[2], 0)
    end
end
return removed
`

// NewLikeFavoriteRedis 创建 LikeFavoriteRedis 实例
func NewLikeFavoriteRedis(rdb *redis.Client) *LikeFavoriteRedis {
	return &LikeFavoriteRedis{rdb: rdb}
}

// Like 点赞，返回是否成功
func (l *LikeFavoriteRedis) Like(ctx context.Context, subject Subject, subjectID, userID int64) (bool, error) {
	setKey := LikeSetKey(subject, subjectID)
	countKey := LikeCountKey(subject, subjectID)

	result, err := l.rdb.Eval(ctx, likeScript, []string{setKey, countKey}, userID).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// Unlike 取消点赞，返回是否成功
func (l *LikeFavoriteRedis) Unlike(ctx context.Context, subject Subject, subjectID, userID int64) (bool, error) {
	setKey := LikeSetKey(subject, subjectID)
	countKey := LikeCountKey(subject, subjectID)

	result, err := l.rdb.Eval(ctx, unlikeScript, []string{setKey, countKey}, userID).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// Collect 收藏，返回是否成功
func (l *LikeFavoriteRedis) Collect(ctx context.Context, subject Subject, subjectID, userID int64) (bool, error) {
	setKey := CollectSetKey(subject, subjectID)
	countKey := CollectCountKey(subject, subjectID)

	result, err := l.rdb.Eval(ctx, likeScript, []string{setKey, countKey}, userID).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// Uncollect 取消收藏，返回是否成功
func (l *LikeFavoriteRedis) Uncollect(ctx context.Context, subject Subject, subjectID, userID int64) (bool, error) {
	setKey := CollectSetKey(subject, subjectID)
	countKey := CollectCountKey(subject, subjectID)

	result, err := l.rdb.Eval(ctx, unlikeScript, []string{setKey, countKey}, userID).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// IsLiked 检查用户是否点赞
func (l *LikeFavoriteRedis) IsLiked(ctx context.Context, subject Subject, subjectID, userID int64) (bool, error) {
	key := LikeSetKey(subject, subjectID)
	return l.rdb.SIsMember(ctx, key, userID).Result()
}

// IsCollected 检查用户是否收藏
func (l *LikeFavoriteRedis) IsCollected(ctx context.Context, subject Subject, subjectID, userID int64) (bool, error) {
	key := CollectSetKey(subject, subjectID)
	return l.rdb.SIsMember(ctx, key, userID).Result()
}

// GetLikeCount 获取点赞数
func (l *LikeFavoriteRedis) GetLikeCount(ctx context.Context, subject Subject, subjectID int64) (int64, bool, error) {
	key := LikeCountKey(subject, subjectID)
	count, err := l.rdb.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return count, true, nil
}

// GetCollectCount 获取收藏数
func (l *LikeFavoriteRedis) GetCollectCount(ctx context.Context, subject Subject, subjectID int64) (int64, bool, error) {
	key := CollectCountKey(subject, subjectID)
	count, err := l.rdb.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return count, true, nil
}

// MGetLikeCounts 批量获取点赞数
func (l *LikeFavoriteRedis) MGetLikeCounts(ctx context.Context, subject Subject, subjectIDs []int64) (map[int64]int64, error) {
	if len(subjectIDs) == 0 {
		return make(map[int64]int64), nil
	}
	keys := make([]string, len(subjectIDs))
	for i, id := range subjectIDs {
		keys[i] = LikeCountKey(subject, id)
	}
	values, err := l.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	result := make(map[int64]int64)
	for i, v := range values {
		if v != nil {
			count, err := strconv.ParseInt(v.(string), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid count for subjectID %d: %w", subjectIDs[i], err)
			}
			result[subjectIDs[i]] = count
		}
	}
	return result, nil
}

// MGetUserLikedStatus 批量检查用户是否点赞多个目标
func (l *LikeFavoriteRedis) MGetUserLikedStatus(ctx context.Context, subject Subject, subjectID int64, userIDs []int64) (map[int64]bool, error) {
	if len(userIDs) == 0 {
		return make(map[int64]bool), nil
	}
	key := LikeSetKey(subject, subjectID)
	pipe := l.rdb.Pipeline()
	cmds := make([]*redis.BoolCmd, len(userIDs))
	for i, uid := range userIDs {
		cmds[i] = pipe.SIsMember(ctx, key, uid)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]bool)
	for i, cmd := range cmds {
		liked, err := cmd.Result()
		if err != nil {
			return nil, fmt.Errorf("SIsMember for userID %d failed: %w", userIDs[i], err)
		}
		result[userIDs[i]] = liked
	}
	return result, nil
}

// SetLikeCount 设置点赞数用于回填和预热
func (l *LikeFavoriteRedis) SetLikeCount(ctx context.Context, subject Subject, subjectID, count int64) error {
	key := LikeCountKey(subject, subjectID)
	return l.rdb.Set(ctx, key, count, 0).Err() // 0 = 永久
}

// SetCollectCount 设置收藏数
func (l *LikeFavoriteRedis) SetCollectCount(ctx context.Context, subject Subject, subjectID, count int64) error {
	key := CollectCountKey(subject, subjectID)
	return l.rdb.Set(ctx, key, count, 0).Err()
}

// AddLikedUser 添加点赞用户到Set用于预热
func (l *LikeFavoriteRedis) AddLikedUser(ctx context.Context, subject Subject, subjectID, userID int64) error {
	key := LikeSetKey(subject, subjectID)
	return l.rdb.SAdd(ctx, key, userID).Err()
}
