package asr

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"ottr/internal/asr/native"
)

// assignSpeakers labels subtitles locally, then maps each cluster to the
// persistent speaker library using its embedding.
func (s *Service) assignSpeakers(wav string, segments []Segment) ([]Segment, error) {
	if len(segments) == 0 {
		return segments, nil
	}
	segmentation, embedding, err := s.models.DiarizationModels()
	if err != nil {
		return segments, nil
	}
	samples, _, err := readWav(wav)
	if err != nil {
		return nil, err
	}
	ranges, err := native.Diarize(segmentation, embedding, samples)
	if err != nil {
		return nil, err
	}
	segments = speakerRanges(segments, ranges)
	s.speakerMu.Lock()
	defer s.speakerMu.Unlock()
	localIDs := make([]int, len(segments))
	for i := range segments {
		localIDs[i] = segments[i].Speaker
	}
	seen := map[int]bool{}
	for i := range segments {
		local := segments[i].Speaker
		if seen[local] {
			continue
		}
		seen[local] = true
		vector, err := speakerVector(embedding, samples, ranges, local)
		if err != nil {
			return nil, err
		}
		id, err := s.matchSpeaker(vector)
		if err != nil {
			return nil, err
		}
		for j := range segments {
			if localIDs[j] == local {
				segments[j].Speaker = id
			}
		}
	}
	return segments, nil
}

func speakerRanges(segments []Segment, ranges []native.SpeakerSegment) []Segment {
	labels := map[int]int{}
	for i := range segments {
		bestSpeaker, bestOverlap := 0, int64(0)
		for _, r := range ranges {
			overlap := min(segments[i].EndMs, r.EndMs) - max(segments[i].StartMs, r.StartMs)
			if overlap > bestOverlap {
				bestSpeaker, bestOverlap = r.Speaker, overlap
			}
		}
		label, ok := labels[bestSpeaker]
		if !ok {
			label = len(labels)
			labels[bestSpeaker] = label
		}
		segments[i].Speaker = label
	}
	return segments
}

func speakerVector(model string, samples []float32, ranges []native.SpeakerSegment, speaker int) ([]float32, error) {
	var audio []float32
	for _, r := range ranges {
		if r.Speaker != speaker {
			continue
		}
		start := max(int64(0), r.StartMs*16)
		end := min(int64(len(samples)), r.EndMs*16)
		if start < end {
			audio = append(audio, samples[start:end]...)
		}
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("speaker %d has no audio", speaker+1)
	}
	return native.SpeakerEmbedding(model, audio)
}

func (s *Service) matchSpeaker(vector []float32) (int, error) {
	db, err := s.store()
	if err != nil {
		return 0, err
	}
	rows, err := db.Query(`SELECT id, embedding FROM speaker`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	bestID, bestScore := 0, 0.65
	for rows.Next() {
		var id int
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return 0, err
		}
		candidate := decodeEmbedding(blob)
		if score := cosine(vector, candidate); score > bestScore {
			bestID, bestScore = id, score
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if bestID != 0 {
		return bestID, nil
	}
	res, err := db.Exec(`INSERT INTO speaker (name, embedding, created_at) VALUES ('', ?, datetime('now'))`, encodeEmbedding(vector))
	if err != nil {
		return 0, err
	}
	id64, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	id := int(id64)
	if _, err := db.Exec(`UPDATE speaker SET name = ? WHERE id = ?`, fmt.Sprintf("Speaker %d", id), id); err != nil {
		return 0, err
	}
	return id, nil
}

func encodeEmbedding(vector []float32) []byte {
	out := make([]byte, len(vector)*4)
	for i, value := range vector {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(value))
	}
	return out
}

func decodeEmbedding(data []byte) []float32 {
	out := make([]float32, len(data)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return out
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, aa, bb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		aa += float64(a[i]) * float64(a[i])
		bb += float64(b[i]) * float64(b[i])
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / math.Sqrt(aa*bb)
}

func (s *Service) speakerNames() (map[int]string, error) {
	db, err := s.store()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id, name FROM speaker`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

func (s *Service) RenameSpeaker(id int, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("speaker name cannot be empty")
	}
	db, err := s.store()
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE speaker SET name = ? WHERE id = ?`, name, id)
	return err
}

// Speaker is one row of the global speaker library, with what the library
// knows about them: on how many recordings they were heard, for how long, and
// when last.
type Speaker struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Recordings int    `json:"recordings"`
	DurationMs int64  `json:"durationMs"`
	LastHeard  string `json:"lastHeard,omitempty"` // the newest recording's date
}

// Speakers lists the library, the speakers heard on most recordings first.
func (s *Service) Speakers() ([]Speaker, error) {
	db, err := s.store()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`
		SELECT sp.id, sp.name, COUNT(x.media_id) AS recordings,
		       COALESCE(SUM(m.duration_ms), 0), COALESCE(MAX(m.created_at), '')
		FROM speaker sp
		LEFT JOIN (
		    SELECT DISTINCT s.speaker AS speaker, t.media_id AS media_id
		    FROM segment s JOIN transcript t ON t.id = s.transcript_id
		    WHERE s.speaker > 0
		) x ON x.speaker = sp.id
		LEFT JOIN media m ON m.id = x.media_id
		GROUP BY sp.id, sp.name
		ORDER BY recordings DESC, sp.name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Speaker
	for rows.Next() {
		var sp Speaker
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.Recordings, &sp.DurationMs, &sp.LastHeard); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// RemoveSpeaker drops one speaker from the library.
func (s *Service) RemoveSpeaker(id int) error {
	db, err := s.store()
	if err != nil {
		return err
	}
	// shortcut: transcripts keep the removed id and fall back to a numbered
	// label; merging two voices into one is what the library still needs.
	_, err = db.Exec(`DELETE FROM speaker WHERE id = ?`, id)
	return err
}
