package audio

import "fmt"

type Decoder interface {
	Decode(packet EncodedPacket) ([]PCMFrame, error)
}

type Factory interface {
	Codec() Codec
	New(packet EncodedPacket) (Decoder, error)
}

type Registry struct {
	factories map[Codec]Factory
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[Codec]Factory),
	}
}

func (r *Registry) Register(factory Factory) {
	if factory == nil {
		return
	}
	r.factories[factory.Codec()] = factory
}

func (r *Registry) Lookup(codec Codec) (Factory, bool) {
	factory, ok := r.factories[codec]
	return factory, ok
}

func (r *Registry) NewDecoder(packet EncodedPacket) (Decoder, error) {
	factory, ok := r.Lookup(packet.Codec)
	if !ok {
		return nil, fmt.Errorf("no decoder registered for codec %s", packet.Codec)
	}
	return factory.New(packet)
}
