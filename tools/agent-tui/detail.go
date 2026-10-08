package main

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// describeEvent renders every populated field of the event. It walks the
// protobuf descriptor rather than a hand-written field list, so new schema
// fields show up without touching the TUI.
func describeEvent(e entry) string {
	ev := e.ev
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s%s  %s%s%s\n\n",
		boldTag(typeColor(ev.GetType())), safeText(ev.GetType()), resetTag,
		colorTag(themeMuted), e.at.Format(time.RFC3339Nano), resetTag)

	msg := ev.ProtoReflect()
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if !msg.Has(fd) {
			continue
		}
		fmt.Fprintf(&b, "%s%-18s%s %s%s%s\n",
			colorTag(themeMuted), string(fd.Name()), resetTag,
			colorTag(themeText), safeText(formatFieldValue(fd, msg.Get(fd))), resetTag)
	}
	return b.String()
}

func formatFieldValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) string {
	switch {
	case fd.IsList():
		list := v.List()
		parts := make([]string, 0, list.Len())
		for i := 0; i < list.Len(); i++ {
			parts = append(parts, formatScalar(fd, list.Get(i)))
		}
		return strings.Join(parts, ", ")
	case fd.IsMap():
		var parts []string
		v.Map().Range(func(k protoreflect.MapKey, mv protoreflect.Value) bool {
			parts = append(parts, k.String()+"="+formatScalar(fd.MapValue(), mv))
			return true
		})
		return strings.Join(parts, ", ")
	default:
		return formatScalar(fd, v)
	}
}

func formatScalar(fd protoreflect.FieldDescriptor, v protoreflect.Value) string {
	switch fd.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return prototext.MarshalOptions{EmitUnknown: false}.Format(v.Message().Interface())
	case protoreflect.EnumKind:
		if desc := fd.Enum().Values().ByNumber(v.Enum()); desc != nil {
			return string(desc.Name())
		}
		return fmt.Sprint(v.Enum())
	case protoreflect.BytesKind:
		return fmt.Sprintf("%d bytes", len(v.Bytes()))
	default:
		return v.String()
	}
}
