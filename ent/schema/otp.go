package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type OTP struct {
	ent.Schema
}

func (OTP) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (OTP) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").
			Unique().
			MaxLen(255).
			NotEmpty(),
		field.String("code").
			Unique().
			MaxLen(255).
			NotEmpty(),
		field.Enum("mode").
			Values("register", "reset").
			Default("register"),
		field.Time("expires_at"),
	}
}

func (OTP) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email").
			StorageKey("idx_otp_email"),

		index.Fields("mode", "created_at").
			Annotations(entsql.DescColumns("created_at")).
			StorageKey("idx_otp_mode_time"),

		index.Fields("expires_at"),
	}
}
