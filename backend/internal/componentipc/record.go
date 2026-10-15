package componentipc

// Record represents one strictly decoded opt-in sequenced transport message.
// Data applies only to KindEvent/KindAlert/KindStatus; Receipt applies only to
// KindAck. No policy-control operations are defined by this schema.
type Record struct {
	Kind    Kind
	Data    Sequenced
	Receipt Ack
}

// DecodeRecord validates a typed body as sequenced data or an ACK receipt.
// A decoded data Payload aliases raw, so copy before retaining the data.
func DecodeRecord(kind Kind, raw []byte) (Record, error) {
	switch kind {
	case KindEvent, KindAlert, KindStatus:
		data, err := DecodeSequenced(kind, raw)
		if err != nil {
			return Record{}, err
		}
		return Record{Kind: kind, Data: data}, nil
	case KindAck:
		receipt, err := DecodeAck(raw)
		if err != nil {
			return Record{}, err
		}
		return Record{Kind: kind, Receipt: receipt}, nil
	default:
		return Record{}, ErrFrameDenied
	}
}

// ReceiveRecordCopy enforces a strict sequenced-body protocol on one
// authenticated session. It returns an owned data payload safe across further
// reads. If the peer sends malformed data, a bad ACK, or a raw heartbeat, the
// session is closed: callers must not attempt to resynchronize a mixed stream.
//
// This method is opt-in: existing Session.Receive and ReceiveCopy preserve
// their legacy raw-body semantics. It does not automatically send ACKs.
func (s *Session) ReceiveRecordCopy() (Record, error) {
	message, err := s.ReceiveCopy()
	if err != nil {
		return Record{}, err
	}
	record, err := DecodeRecord(message.Kind, message.Payload)
	if err != nil {
		_ = s.Close()
		return Record{}, err
	}
	return record, nil
}
