package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// MQTT Packet Type Constants
const (
	MQTT_PUBLISH byte = 0x03 // PUBLISH Packet Type
)

// encodeLength encodes MQTT Variable Byte Integer format.
func encodeLength(length int) []byte {
	var encoded []byte
	for {
		b := byte(length % 128) // Extract lower 7 bits
		length /= 128
		if length > 0 {
			b |= 0x80 // Set MSB if more bytes follow
		}
		encoded = append(encoded, b)
		if length == 0 {
			break
		}
	}
	return encoded
}

// decodeLength decodes MQTT Variable Byte Integer format.
func decodeLength(buf *bytes.Buffer) (int, error) {
	multiplier := 1
	value := 0
	for i := 0; i < 4; i++ { // Max 4 bytes
		b, err := buf.ReadByte()
		if err != nil {
			return 0, err
		}
		value += int(b&127) * multiplier // Extract 7-bit value
		multiplier *= 128
		if b&128 == 0 { // If MSB is 0, stop reading
			break
		}
	}
	return value, nil
}

// encodePublishPacket constructs an MQTT PUBLISH packet.
func encodePublishPacket(topic string, payload string, qos byte, retain bool) []byte {
	var buf bytes.Buffer

	// First byte (Fixed Header)
	firstByte := (MQTT_PUBLISH << 4) | (qos << 1) | boolToByte(retain)
	buf.WriteByte(firstByte)

	// Variable Header: Topic Length + Topic Name
	topicBytes := []byte(topic)
	binary.Write(&buf, binary.BigEndian, uint16(len(topicBytes))) // Topic length (2 bytes)
	buf.Write(topicBytes)                                         // Topic Name

	// Packet Identifier (only if QoS > 0)
	if qos > 0 {
		buf.WriteByte(0x00) // Packet ID MSB
		buf.WriteByte(0x01) // Packet ID LSB
	}

	// Payload
	payloadBytes := []byte(payload)

	// Remaining Length (calculated as everything after the first byte)
	remainingLength := len(buf.Bytes()) + len(payloadBytes)
	bufWithLength := bytes.Buffer{}
	bufWithLength.WriteByte(firstByte) // Re-add Fixed Header
	bufWithLength.Write(encodeLength(remainingLength))
	bufWithLength.Write(buf.Bytes())  // Variable Header
	bufWithLength.Write(payloadBytes) // Payload

	return bufWithLength.Bytes()
}

// decodePublishPacket parses an MQTT PUBLISH packet.
func decodePublishPacket(packet []byte) {
	buf := bytes.NewBuffer(packet)

	// Read First Byte (Fixed Header)
	firstByte, _ := buf.ReadByte()
	packetType := (firstByte >> 4) & 0x0F
	qos := (firstByte >> 1) & 0x03
	retain := firstByte & 0x01

	// Read Remaining Length
	remainingLength, _ := decodeLength(buf)

	// Read Topic Length
	var topicLength uint16
	binary.Read(buf, binary.BigEndian, &topicLength)

	// Read Topic Name
	topicBytes := make([]byte, topicLength)
	buf.Read(topicBytes)
	topic := string(topicBytes)

	// Read Packet Identifier (only for QoS > 0)
	packetID := 0
	if qos > 0 {
		packetIDMSB, _ := buf.ReadByte()
		packetIDLSB, _ := buf.ReadByte()
		packetID = int(packetIDMSB)<<8 | int(packetIDLSB)
	}

	// Read Payload
	payloadBytes := buf.Bytes()
	payload := string(payloadBytes)

	// Print Decoded Packet
	fmt.Println("📦 Decoded MQTT Packet:")
	fmt.Println("- Packet Type: ", packetType)
	fmt.Println("- QoS Level:", qos)
	fmt.Println("- Retain Flag:", retain)
	fmt.Println("- Remaining Length:", remainingLength)
	fmt.Println("- Topic:", topic)
	if qos > 0 {
		fmt.Println("- Packet ID:", packetID)
	}
	fmt.Println("- Payload:", payload)
}

// Helper function: Converts bool to byte
func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func main() {
	// Example: Encode a PUBLISH packet
	topic := "test/topic"
	payload := "Hello MQTT"
	qos := byte(1)  // QoS 1
	retain := false // Do not retain

	encodedPacket := encodePublishPacket(topic, payload, qos, retain)
	fmt.Println("🔧 Encoded MQTT Packet:", encodedPacket)

	// Example: Decode the packet
	decodePublishPacket(encodedPacket)
}
