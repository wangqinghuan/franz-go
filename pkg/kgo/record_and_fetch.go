package kgo

import (
	"reflect"
	"time"
	"unsafe"
)

// RecordHeader contains extra information that can be sent with Records.
type RecordHeader struct {
	Key   string
	Value []byte
}

// RecordAttrs contains additional meta information about a record, such as its
// compression or timestamp type.
type RecordAttrs struct {
	// 6 bits are used right now for record batches, and we use the high
	// bit to signify no timestamp due to v0 message set.
	//
	// bits 1 thru 3:
	//   000 no compression
	//   001 gzip
	//   010 snappy
	//   011 lz4
	//   100 zstd
	// bit 4: timestamp type
	// bit 5: is transactional
	// bit 6: is control
	// bit 8: no timestamp type
	attrs uint8
}

// TimestampType specifies how Timestamp was determined.
//
// The default, 0, means that the timestamp was determined in a client
// when the record was produced.
//
// An alternative is 1, which is when the Timestamp is set in Kafka.
//
// Records pre 0.10.0 did not have timestamps and have value -1.
func (a RecordAttrs) TimestampType() int8 {
	if a.attrs&0b1000_0000 != 0 {
		return -1
	}
	return int8(a.attrs & 0b0000_1000)
}

// CompressionType signifies with which algorithm this record was compressed.
//
// 0 is no compression, 1 is gzip, 2 is snappy, 3 is lz4, and 4 is zstd.
func (a RecordAttrs) CompressionType() uint8 {
	return a.attrs & 0b0000_0111
}

// IsTransactional returns whether a record is a part of a transaction.
func (a RecordAttrs) IsTransactional() bool {
	return a.attrs&0b0001_0000 != 0
}

// IsControl returns whether a record is a "control" record (ABORT or COMMIT).
// These are generally not visible unless explicitly opted into.
func (a RecordAttrs) IsControl() bool {
	return a.attrs&0b0010_0000 != 0
}

// Record is a record to write to Kafka.
type Record struct {
	// Key is an optional field that can be used for partition assignment.
	//
	// This is generally used with a hash partitioner to cause all records
	// with the same key to go to the same partition.
	Key []byte
	// Value is blob of data to write to Kafka.
	Value []byte

	// Headers are optional key/value pairs that are passed along with
	// records.
	//
	// These are purely for producers and consumers; Kafka does not look at
	// this field and only writes it to disk.
	Headers []RecordHeader

	// NOTE: if logAppendTime, timestamp is MaxTimestamp, not first + delta
	// zendesk/ruby-kafka#706

	// Timestamp is the timestamp that will be used for this record.
	//
	// Record batches are always written with "CreateTime", meaning that
	// timestamps are generated by clients rather than brokers.
	//
	// This field is always set in Produce.
	Timestamp time.Time

	// Topic is the topic that a record is written to.
	//
	// This must be set for producing.
	Topic string

	// Partition is the partition that a record is written to.
	//
	// For producing, this is left unset. This will be set by the client
	// as appropriate.
	Partition int32

	// Attrs specifies what attributes were on this record.
	Attrs RecordAttrs

	// ProducerEpoch is the producer epoch of this message if it was
	// produced with a producer ID. An epoch and ID of 0 means it was not.
	//
	// For producing, this is left unset. This will be set by the client
	// as appropriate.
	ProducerEpoch int16

	// ProducerEpoch is the producer ID of this message if it was produced
	// with a producer ID. An epoch and ID of 0 means it was not.
	//
	// For producing, this is left unset. This will be set by the client
	// as appropriate.
	ProducerID int64

	// LeaderEpoch is the leader epoch of the broker at the time this
	// record was written, or -1 if on message sets.
	LeaderEpoch int32

	// Offset is the offset that a record is written as.
	//
	// For producing, this is left unset. This will be set by the client as
	// appropriate. If you are producing with no acks, this will just be
	// the offset used in the produce request and does not mirror the
	// offset actually stored within Kafka.
	Offset int64
}

// StringRecord returns a Record with the Value field set to the input value
// string. For producing, this function is useful in tandem with the
// client-level ProduceTopic option.
//
// This function uses the 'unsafe' package to avoid copying value into a slice.
//
// NOTE: It is NOT SAFE to modify the record's value. This function should only
// be used if you only ever read record fields. This function can safely be used
// for producing; the client never modifies a record's key nor value fields.
func StringRecord(value string) *Record {
	var slice []byte
	slicehdr := (*reflect.SliceHeader)(unsafe.Pointer(&slice))
	slicehdr.Data = ((*reflect.StringHeader)(unsafe.Pointer(&value))).Data
	slicehdr.Len = len(value)
	slicehdr.Cap = len(value)

	return &Record{Value: slice}
}

// KeyStringRecord returns a Record with the Key and Value fields set to the
// input key and value strings. For producing, this function is useful in
// tandem with the client-level ProduceTopic option.
//
// This function uses the 'unsafe' package to avoid copying value into a slice.
//
// NOTE: It is NOT SAFE to modify the record's value. This function should only
// be used if you only ever read record fields. This function can safely be used
// for producing; the client never modifies a record's key nor value fields.
func KeyStringRecord(key, value string) *Record {
	r := StringRecord(value)

	keyhdr := (*reflect.SliceHeader)(unsafe.Pointer(&r.Key))
	keyhdr.Data = ((*reflect.StringHeader)(unsafe.Pointer(&key))).Data
	keyhdr.Len = len(key)
	keyhdr.Cap = len(key)

	return r
}

// SliceRecord returns a Record with the Value field set to the input value
// slice. For producing, this function is useful in tandem with the
// client-level ProduceTopic option.
func SliceRecord(value []byte) *Record {
	return &Record{Value: value}
}

// KeySliceRecord returns a Record with the Key and Value fields set to the
// input key and value slices. For producing, this function is useful in
// tandem with the client-level ProduceTopic option.
func KeySliceRecord(key, value []byte) *Record {
	return &Record{Key: key, Value: value}
}

// FetchPartition is a response for a partition in a fetched topic from a
// broker.
type FetchPartition struct {
	// Partition is the partition this is for.
	Partition int32
	// Err is an error for this partition in the fetch.
	//
	// Note that if this is a fatal error, such as data loss or non
	// retriable errors, this partition will never be fetched again.
	Err error
	// HighWatermark is the current high watermark for this partition, that
	// is, the current offset that is on all in sync replicas.
	HighWatermark int64
	// LastStableOffset is the offset at which all prior offsets have been
	// "decided". Non transactional records are always decided immediately,
	// but transactional records are only decided once they are commited or
	// aborted.
	//
	// The LastStableOffset will always be at or under the HighWatermark.
	LastStableOffset int64
	// LogStartOffset is the low watermark of this partition, otherwise
	// known as the earliest offset in the partition.
	LogStartOffset int64
	// Records contains feched records for this partition.
	Records []*Record
}

// FetchTopic is a response for a fetched topic from a broker.
type FetchTopic struct {
	// Topic is the topic this is for.
	Topic string
	// Partitions contains individual partitions in the topic that were
	// fetched.
	Partitions []FetchPartition
}

// Fetch is an individual response from a broker.
type Fetch struct {
	// Topics are all topics being responded to from a fetch to a broker.
	Topics []FetchTopic
}

// Fetches is a group of fetches from brokers.
type Fetches []Fetch

// FetchError is an error in a fetch along with the topic and partition that
// the error was on.
type FetchError struct {
	Topic     string
	Partition int32
	Err       error
}

// Errors returns all errors in a fetch with the topic and partition that
// errored.
//
// There are three classes of errors possible:
//
//   1) a normal kerr.Error; these are usually the non-retriable kerr.Errors,
//      but theoretically a non-retriable error can be fixed at runtime (auth
//      error? fix auth). It is worth restarting the client for these errors if
//      you do not intend to fix this problem at runtime.
//
//   2) an injected *ErrDataLoss; these are informational, the client
//      automatically resets consuming to where it should and resumes. This
//      error is worth logging and investigating, but not worth restarting the
//      client for.
//
//   3) an untyped batch parse failure; these are usually unrecoverable by
//      restarts, and it may be best to just let the client continue. However,
//      restarting is an option, but you may need to manually repair your
//      partition.
//
func (fs Fetches) Errors() []FetchError {
	var errs []FetchError
	fs.EachErr(func(t string, p int32, err error) {
		errs = append(errs, FetchError{t, p, err})
	})
	return errs
}

// EachErr calls fn for every partition that had a fetch error with the
// topic, partition, and error.
//
// This function has the same semantics as the Errors function; refer to the
// documentation on that function for what types of errors are possible.
func (fs Fetches) EachErr(fn func(string, int32, error)) {
	for _, f := range fs {
		for _, ft := range f.Topics {
			for _, fp := range ft.Partitions {
				if fp.Err != nil {
					fn(ft.Topic, fp.Partition, fp.Err)
				}
			}
		}
	}
}

// RecordIter returns an iterator over all records in a fetch.
//
// Note that errors should be inspected as well.
func (fs Fetches) RecordIter() *FetchesRecordIter {
	iter := &FetchesRecordIter{fetches: fs}
	iter.prepareNext()
	return iter
}

// FetchesRecordIter iterates over records in a fetch.
type FetchesRecordIter struct {
	fetches []Fetch
	ti      int // index to current topic in fetches[0]
	pi      int // index to current partition in current topic
	ri      int // index to current record in current partition
}

// Done returns whether there are any more records to iterate over.
func (i *FetchesRecordIter) Done() bool {
	return len(i.fetches) == 0
}

// Next returns the next record from a fetch.
func (i *FetchesRecordIter) Next() *Record {
	next := i.fetches[0].Topics[i.ti].Partitions[i.pi].Records[i.ri]
	i.ri++
	i.prepareNext()
	return next
}

func (i *FetchesRecordIter) prepareNext() {
beforeFetch0:
	if len(i.fetches) == 0 {
		return
	}

	fetch0 := &i.fetches[0]
beforeTopic:
	if i.ti >= len(fetch0.Topics) {
		i.fetches = i.fetches[1:]
		i.ti = 0
		goto beforeFetch0
	}

	topic := &fetch0.Topics[i.ti]
beforePartition:
	if i.pi >= len(topic.Partitions) {
		i.ti++
		i.pi = 0
		goto beforeTopic
	}

	partition := &topic.Partitions[i.pi]
	if i.ri >= len(partition.Records) {
		i.pi++
		i.ri = 0
		goto beforePartition
	}
}

// EachPartition calls fn for each partition in Fetches.
//
// Partitions are not visited in any specific order, and a topic may be visited
// multiple times if it is spread across fetches.
func (fs Fetches) EachPartition(fn func(FetchTopicPartition)) {
	for _, fetch := range fs {
		for _, topic := range fetch.Topics {
			for i := range topic.Partitions {
				fn(FetchTopicPartition{
					Topic:     topic.Topic,
					Partition: topic.Partitions[i],
				})
			}
		}
	}
}

// EachTopic calls fn for each topic in Fetches.
//
// This is a convenience function that groups all partitions for the same topic
// from many fetches into one FetchTopic. A map is internally allocated to
// group partitions per topic before calling fn.
func (fs Fetches) EachTopic(fn func(FetchTopic)) {
	switch len(fs) {
	case 0:
		return
	case 1:
		for _, topic := range fs[0].Topics {
			fn(topic)
		}
		return
	}

	topics := make(map[string][]FetchPartition)
	for _, fetch := range fs {
		for _, topic := range fetch.Topics {
			topics[topic.Topic] = append(topics[topic.Topic], topic.Partitions...)
		}
	}

	for topic, partitions := range topics {
		fn(FetchTopic{
			topic,
			partitions,
		})
	}
}

// EachRecord calls fn for each record in Fetches.
//
// This is very similar to using a record iter, and is solely a convenience
// function depending on which style you prefer.
func (fs Fetches) EachRecord(fn func(*Record)) {
	for iter := fs.RecordIter(); !iter.Done(); {
		fn(iter.Next())
	}
}

// FetchTopicPartition is similar to FetchTopic, but for an individual
// partition.
type FetchTopicPartition struct {
	// Topic is the topic this is for.
	Topic string
	// Partition is an individual partition within this topic.
	Partition FetchPartition
}

// EachRecord calls fn for each record in the topic's partition.
func (r *FetchTopicPartition) EachRecord(fn func(*Record)) {
	for _, r := range r.Partition.Records {
		fn(r)
	}
}
