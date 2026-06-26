package app

type SingBoxOutboundBuilderForTest struct {
	builder *singBoxOutboundBuilder
}

func NewSingBoxOutboundBuilderForTest() (*SingBoxOutboundBuilderForTest, error) {
	builder, err := newSingBoxOutboundBuilder()
	if err != nil {
		return nil, err
	}
	return &SingBoxOutboundBuilderForTest{builder: builder}, nil
}

func (b *SingBoxOutboundBuilderForTest) ParseForTest(outboundJSON string) error {
	raw, err := runtimeOutboundJSON(outboundJSON, "test-outbound", "")
	if err != nil {
		return err
	}
	_, err = b.builder.parseOutbound(b.builder.ctx, raw)
	return err
}

func (b *SingBoxOutboundBuilderForTest) Close() error {
	if b == nil || b.builder == nil {
		return nil
	}
	return b.builder.Close()
}
