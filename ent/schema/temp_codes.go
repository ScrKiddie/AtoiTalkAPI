package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type TempCodes struct {
	ent.Schema
}

func (TempCodes) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (TempCodes) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").
			Unique().
			MaxLen(255).
			NotEmpty(),
		field.String("code").
			MaxLen(255).
			NotEmpty(),
		field.Enum("mode").
			Values("register", "reset").
			Default("register"),
		field.Time("expires_at"),
	}
}

func (TempCodes) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email").
			StorageKey("idx_temp_codes_email"),

		index.Fields("mode", "created_at").
			Annotations(entsql.DescColumns("created_at")).
			StorageKey("idx_temp_codes_mode_time"),

		index.Fields("expires_at"),
	}
}
