package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/hotel"
)

type hotelInventoryLLM struct {
	base  adapters.LLMAdapter
	store *hotel.Store
}

func newHotelInventoryLLM(base adapters.LLMAdapter, store *hotel.Store) adapters.LLMAdapter {
	if base == nil || store == nil {
		return base
	}
	return &hotelInventoryLLM{
		base:  base,
		store: store,
	}
}

func (l *hotelInventoryLLM) Name() string {
	return l.base.Name()
}

func (l *hotelInventoryLLM) Complete(ctx context.Context, req adapters.CompletionRequest) (<-chan adapters.LLMEvent, error) {
	rooms := l.store.ListRoomTypes()
	if len(rooms) == 0 {
		return l.base.Complete(ctx, req)
	}

	next := req
	next.Text = buildHotelInventoryPrompt(rooms, req.Text)
	return l.base.Complete(ctx, next)
}

func buildHotelInventoryPrompt(rooms []hotel.RoomType, transcript string) string {
	var b strings.Builder
	b.WriteString("请基于以下实时酒店库存回答用户，库存是唯一事实来源。")
	b.WriteString("不要声称已经查询到与下列数据相反的结果；只有 available_count 为 0 时才能说售罄。")
	b.WriteString("如果用户询问或选择某个房型，请直接按这里的库存数量回答。\n\n")
	b.WriteString("实时库存:\n")
	for _, room := range rooms {
		fmt.Fprintf(
			&b,
			"- room_type_id=%s, name=%s, price=%s, capacity=%d, available_count=%d, description=%s\n",
			room.ID,
			room.Name,
			room.PriceLabel,
			room.Capacity,
			room.AvailableCount,
			room.Description,
		)
	}
	b.WriteString("\n用户刚才说:\n")
	b.WriteString(transcript)
	return b.String()
}
