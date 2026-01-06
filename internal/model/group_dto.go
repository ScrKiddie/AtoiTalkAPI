package model

import "mime/multipart"

type CreateGroupChatRequest struct {
	Name        string                `form:"name" validate:"required,min=3,max=100"`
	Description string                `form:"description" validate:"max=255"`
	MemberIDs   []int                 `form:"member_ids" validate:"required,min=1,dive,gt=0"`
	Avatar      *multipart.FileHeader `form:"avatar" validate:"omitempty"`
}
