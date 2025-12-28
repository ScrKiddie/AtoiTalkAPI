package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Media struct {
	ent.Schema
}

func (Media) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (Media) Fields() []ent.Field {
	return []ent.Field{
		field.String("file_name").Unique().MaxLen(255).NotEmpty(),
		field.String("original_name").MaxLen(255).NotEmpty(),
		field.Int64("file_size").Positive(),
		field.String("mime_type").MaxLen(100).NotEmpty(),
		field.Enum("status").Values("pending", "active", "failed").Default("pending"),
		field.Int("message_id").Optional().Nillable(),
	}
}
func (Media) Edges() []ent.Edge {
	return []ent.Edge{

		edge.From("message", Message.Type).
			Ref("attachments").
			Field("message_id").
			Unique(),

		edge.To("user_avatar", User.Type).
			Unique(),
		edge.To("group_avatar", GroupChat.Type).
			Unique(),
	}
}
