package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Attachment struct {
	ent.Schema
}

func (Attachment) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (Attachment) Fields() []ent.Field {
	return []ent.Field{
		field.Int("message_id").Optional().Nillable(),
		field.String("file_name").MaxLen(255).NotEmpty(),
		field.Enum("file_type").Values("image", "video", "audio", "document", "code", "archive", "executable", "other"),
		field.String("original_name").MaxLen(255).NotEmpty(),
		field.Int64("file_size").Positive(),
	}
}

func (Attachment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("message", Message.Type).Ref("attachments").Field("message_id").Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (Attachment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("message_id").Annotations(entsql.IndexWhere("message_id IS NOT NULL")),
	}
}
