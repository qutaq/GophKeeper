package grpcserver

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/qutaq/gophkeeper/internal/model"
	gophkeeperv1 "github.com/qutaq/gophkeeper/internal/proto"
)

func toProtoItem(item *model.Item) *gophkeeperv1.Item {
	if item == nil {
		return nil
	}
	meta := map[string]string(item.Metadata)
	if meta == nil {
		meta = map[string]string{}
	}
	b := gophkeeperv1.Item_builder{
		Id:            item.ID,
		OwnerId:       item.OwnerID,
		Type:          gophkeeperv1.DataType(item.Type),
		EncryptedData: append([]byte(nil), item.EncryptedData...),
		Metadata:      meta,
		Version:       item.Version,
		Deleted:       item.Deleted,
	}
	if !item.UpdatedAt.IsZero() {
		b.UpdatedAt = timestamppb.New(item.UpdatedAt)
	}
	return b.Build()
}

func toProtoItems(items []model.Item) []*gophkeeperv1.Item {
	out := make([]*gophkeeperv1.Item, 0, len(items))
	for i := range items {
		out = append(out, toProtoItem(&items[i]))
	}
	return out
}

func fromProtoMetadata(m map[string]string) model.Metadata {
	if m == nil {
		return model.Metadata{}
	}
	out := make(model.Metadata, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
