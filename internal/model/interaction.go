package model

// UserActivityInteraction 用户对活动的互动（点赞/收藏）
type UserActivityInteraction struct {
	Id         int64 `gorm:"primaryKey;type:bigint;column:id"`
	UserId     int64 `gorm:"type:bigint;index;column:user_id;not null"`
	ActivityId int64 `gorm:"type:bigint;index;column:activity_id;not null"`
	Type       string `gorm:"type:varchar(20);column:type;not null"` // like/collect
}

// UserPostInteraction 用户对帖子的互动
type UserPostInteraction struct {
	Id     int64 `gorm:"primaryKey;type:bigint;column:id"`
	UserId int64 `gorm:"type:bigint;index;column:user_id;not null"`
	PostId int64 `gorm:"type:bigint;index;column:post_id;not null"`
	Type   string `gorm:"type:varchar(20);column:type;not null"` // like/collect
}

// UserCommentInteraction 用户对评论的互动
type UserCommentInteraction struct {
	Id        int64 `gorm:"primaryKey;type:bigint;column:id"`
	UserId    int64 `gorm:"type:bigint;index;column:user_id;not null"`
	CommentId int64 `gorm:"type:bigint;index;column:comment_id;not null"`
	Type      string `gorm:"type:varchar(20);column:type;not null"` // like
}