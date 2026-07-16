package bounds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ManifestVersion1               = 1
	ManifestVersion2               = 2
	gameObjectAssetPathRequirement = "must be an Assets path with a supported GameObject asset extension (.prefab, .fbx, .dae, .3ds, .dxf, .obj, .skp, .blend, .max, .ma, or .mb)"
)

type Vec3 [3]float64

type AABB struct {
	Center Vec3 `json:"center"`
	Size   Vec3 `json:"size"`
}

type Quat [4]float64

type OBB struct {
	ID       string `json:"id"`
	Center   Vec3   `json:"center"`
	Size     Vec3   `json:"size"`
	Rotation Quat   `json:"rotation"`
}

type ContactFrame struct {
	ID      string     `json:"id"`
	Point   Vec3       `json:"point"`
	Normal  Vec3       `json:"normal"`
	Tangent Vec3       `json:"tangent"`
	Size    [2]float64 `json:"size"`
}

// ContactRequirement carries the reviewed physical policy associated with a
// named contact frame. Detailed scans do not invent these values; they are
// populated only when an approved spatial contract is overlaid.
type ContactRequirement struct {
	ID                 string  `json:"id"`
	Kind               string  `json:"kind"`
	FrameID            string  `json:"frame_id"`
	Target             string  `json:"target"`
	MinimumGap         float64 `json:"minimum_gap"`
	MaximumGap         float64 `json:"maximum_gap"`
	MaximumPenetration float64 `json:"maximum_penetration"`
	MinimumSupport     float64 `json:"minimum_support"`
	DirectionAlignment float64 `json:"direction_alignment"`
}

type SpatialProfile struct {
	OBBs        []OBB `json:"obbs"`
	Forward     Vec3  `json:"forward"`
	Up          Vec3  `json:"up"`
	PivotOffset Vec3  `json:"pivot_offset"`
	// Frames is the canonical collection for reviewed, arbitrarily named
	// contact frames. The three legacy fields remain readable/writable so v2
	// manifests produced before this collection was introduced keep working.
	Frames         []ContactFrame       `json:"frames,omitempty"`
	BottomContact  *ContactFrame        `json:"bottom_contact,omitempty"`
	BackContact    *ContactFrame        `json:"back_contact,omitempty"`
	TopContact     *ContactFrame        `json:"top_contact,omitempty"`
	Contacts       []ContactRequirement `json:"contacts,omitempty"`
	Source         string               `json:"source"`
	Confidence     float64              `json:"confidence"`
	Reviewed       bool                 `json:"reviewed"`
	DependencyHash string               `json:"dependency_hash,omitempty"`
}

type SurfacePatch struct {
	ID        string     `json:"id"`
	Type      string     `json:"type"`
	Origin    Vec3       `json:"origin"`
	Normal    Vec3       `json:"normal"`
	Tangent   Vec3       `json:"tangent"`
	Size      [2]float64 `json:"size"`
	Reviewed  bool       `json:"reviewed"`
	Supported bool       `json:"supported"`
	Reason    string     `json:"reason,omitempty"`
}

type ObjectBounds struct {
	FileID  int64           `json:"fileID"`
	Name    string          `json:"name"`
	Bounds  AABB            `json:"bounds"`
	Spatial *SpatialProfile `json:"spatial,omitempty"`
}

type PrefabBounds struct {
	Path    string          `json:"path"`
	Bounds  AABB            `json:"bounds"`
	GUID    string          `json:"guid,omitempty"`
	Spatial *SpatialProfile `json:"spatial,omitempty"`
}

type Manifest struct {
	Scene        string         `json:"scene"`
	Source       string         `json:"source"`
	Version      int            `json:"version"`
	Objects      []ObjectBounds `json:"objects"`
	Prefabs      []PrefabBounds `json:"prefabs"`
	Capabilities []string       `json:"capabilities,omitempty"`
	Surfaces     []SurfacePatch `json:"surfaces,omitempty"`
}

type rawManifest struct {
	Scene   string          `json:"scene"`
	Source  string          `json:"source"`
	Version int             `json:"version"`
	Objects json.RawMessage `json:"objects"`
	Prefabs json.RawMessage `json:"prefabs"`
}

type rawObject struct {
	FileID int64           `json:"fileID"`
	Name   string          `json:"name"`
	Bounds json.RawMessage `json:"bounds"`
}

type rawPrefab struct {
	Path   string          `json:"path"`
	Bounds json.RawMessage `json:"bounds"`
}

type rawAABB struct {
	Center json.RawMessage `json:"center"`
	Size   json.RawMessage `json:"size"`
}

func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}

	manifest, err := decodeManifest(data)
	if err != nil {
		return Manifest{}, err
	}

	return manifest, nil
}

func Save(path string, manifest Manifest) error {
	if err := validateManifest(manifest); err != nil {
		return err
	}

	normalized := Manifest{
		Scene:        manifest.Scene,
		Source:       manifest.Source,
		Version:      manifest.Version,
		Objects:      append([]ObjectBounds(nil), manifest.Objects...),
		Prefabs:      append([]PrefabBounds(nil), manifest.Prefabs...),
		Capabilities: append([]string(nil), manifest.Capabilities...),
		Surfaces:     append([]SurfacePatch(nil), manifest.Surfaces...),
	}

	sort.Slice(normalized.Objects, func(i, j int) bool {
		return normalized.Objects[i].FileID < normalized.Objects[j].FileID
	})
	sort.Slice(normalized.Prefabs, func(i, j int) bool {
		return normalized.Prefabs[i].Path < normalized.Prefabs[j].Path
	})
	sort.Strings(normalized.Capabilities)
	sort.Slice(normalized.Surfaces, func(i, j int) bool { return normalized.Surfaces[i].ID < normalized.Surfaces[j].ID })
	for i := range normalized.Objects {
		if normalized.Objects[i].Spatial != nil {
			sort.Slice(normalized.Objects[i].Spatial.OBBs, func(a, b int) bool {
				return normalized.Objects[i].Spatial.OBBs[a].ID < normalized.Objects[i].Spatial.OBBs[b].ID
			})
			sort.Slice(normalized.Objects[i].Spatial.Frames, func(a, b int) bool {
				return normalized.Objects[i].Spatial.Frames[a].ID < normalized.Objects[i].Spatial.Frames[b].ID
			})
			sort.Slice(normalized.Objects[i].Spatial.Contacts, func(a, b int) bool {
				return normalized.Objects[i].Spatial.Contacts[a].ID < normalized.Objects[i].Spatial.Contacts[b].ID
			})
		}
	}
	for i := range normalized.Prefabs {
		if normalized.Prefabs[i].Spatial != nil {
			sort.Slice(normalized.Prefabs[i].Spatial.OBBs, func(a, b int) bool {
				return normalized.Prefabs[i].Spatial.OBBs[a].ID < normalized.Prefabs[i].Spatial.OBBs[b].ID
			})
			sort.Slice(normalized.Prefabs[i].Spatial.Frames, func(a, b int) bool {
				return normalized.Prefabs[i].Spatial.Frames[a].ID < normalized.Prefabs[i].Spatial.Frames[b].ID
			})
			sort.Slice(normalized.Prefabs[i].Spatial.Contacts, func(a, b int) bool {
				return normalized.Prefabs[i].Spatial.Contacts[a].ID < normalized.Prefabs[i].Spatial.Contacts[b].ID
			})
		}
	}

	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}

	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func decodeManifest(data []byte) (Manifest, error) {
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return Manifest{}, fmt.Errorf("invalid manifest: %w", err)
	}
	if header.Version == ManifestVersion2 {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		var manifest Manifest
		if err := decoder.Decode(&manifest); err != nil {
			return Manifest{}, fmt.Errorf("invalid manifest: %w", err)
		}
		if err := ensureEOF(decoder); err != nil {
			return Manifest{}, fmt.Errorf("invalid manifest: %w", err)
		}
		if err := validateManifest(manifest); err != nil {
			return Manifest{}, err
		}
		return manifest, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var raw rawManifest
	if err := decoder.Decode(&raw); err != nil {
		return Manifest{}, fmt.Errorf("invalid manifest: %w", err)
	}

	if err := ensureEOF(decoder); err != nil {
		return Manifest{}, fmt.Errorf("invalid manifest: %w", err)
	}

	if len(raw.Objects) == 0 {
		return Manifest{}, fmt.Errorf("invalid manifest: missing objects")
	}
	if len(raw.Prefabs) == 0 {
		return Manifest{}, fmt.Errorf("invalid manifest: missing prefabs")
	}

	objectMessages, err := decodeArray(raw.Objects, "objects")
	if err != nil {
		return Manifest{}, err
	}

	prefabMessages, err := decodeArray(raw.Prefabs, "prefabs")
	if err != nil {
		return Manifest{}, err
	}

	manifest := Manifest{
		Scene:   raw.Scene,
		Source:  raw.Source,
		Version: raw.Version,
		Objects: make([]ObjectBounds, 0, len(objectMessages)),
		Prefabs: make([]PrefabBounds, 0, len(prefabMessages)),
	}

	for i, data := range objectMessages {
		prefix := fmt.Sprintf("objects[%d]", i)
		var object rawObject
		if err := decodeStrictJSON(data, &object); err != nil {
			return Manifest{}, wrapNestedDecodeError(prefix, err)
		}
		if len(object.Bounds) == 0 || bytes.Equal(bytes.TrimSpace(object.Bounds), []byte("null")) {
			return Manifest{}, fmt.Errorf("invalid manifest: missing %s.bounds", prefix)
		}

		bounds, err := decodeBounds(object.Bounds, prefix+".bounds")
		if err != nil {
			return Manifest{}, err
		}

		manifest.Objects = append(manifest.Objects, ObjectBounds{
			FileID: object.FileID,
			Name:   object.Name,
			Bounds: bounds,
		})
	}

	for i, data := range prefabMessages {
		prefix := fmt.Sprintf("prefabs[%d]", i)
		var prefab rawPrefab
		if err := decodeStrictJSON(data, &prefab); err != nil {
			return Manifest{}, wrapNestedDecodeError(prefix, err)
		}
		if len(prefab.Bounds) == 0 || bytes.Equal(bytes.TrimSpace(prefab.Bounds), []byte("null")) {
			return Manifest{}, fmt.Errorf("invalid manifest: missing %s.bounds", prefix)
		}

		bounds, err := decodeBounds(prefab.Bounds, prefix+".bounds")
		if err != nil {
			return Manifest{}, err
		}

		manifest.Prefabs = append(manifest.Prefabs, PrefabBounds{
			Path:   prefab.Path,
			Bounds: bounds,
		})
	}

	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}

	return manifest, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return fmt.Errorf("unexpected trailing JSON content")
}

func decodeArray(data json.RawMessage, path string) ([]json.RawMessage, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, fmt.Errorf("invalid manifest: %s must be an array", path)
	}

	var values []json.RawMessage
	if err := decodeStrictJSON(data, &values); err != nil {
		return nil, fmt.Errorf("invalid manifest: %s must be an array", path)
	}

	return values, nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	return ensureEOF(decoder)
}

func wrapNestedDecodeError(path string, err error) error {
	const prefix = "json: unknown field "
	if strings.HasPrefix(err.Error(), prefix) {
		field := strings.TrimPrefix(err.Error(), prefix)
		return fmt.Errorf("invalid manifest: %s: unknown field %s", path, field)
	}

	return fmt.Errorf("invalid manifest: %s is invalid", path)
}

func decodeBounds(data json.RawMessage, path string) (AABB, error) {
	var raw rawAABB
	if err := decodeStrictJSON(data, &raw); err != nil {
		return AABB{}, wrapNestedDecodeError(path, err)
	}

	return decodeAABB(raw, path)
}

func decodeAABB(raw rawAABB, prefix string) (AABB, error) {
	center, err := decodeVec3(raw.Center, prefix+".center")
	if err != nil {
		return AABB{}, err
	}

	size, err := decodeVec3(raw.Size, prefix+".size")
	if err != nil {
		return AABB{}, err
	}

	return AABB{
		Center: center,
		Size:   size,
	}, nil
}

func decodeVec3(data json.RawMessage, path string) (Vec3, error) {
	if len(data) == 0 {
		return Vec3{}, fmt.Errorf("invalid manifest: missing %s", path)
	}

	var values []float64
	if err := json.Unmarshal(data, &values); err != nil {
		return Vec3{}, fmt.Errorf("invalid manifest: %s must be an array of numbers", path)
	}
	if len(values) != 3 {
		return Vec3{}, fmt.Errorf("invalid manifest: %s must have exactly 3 numbers", path)
	}

	return Vec3{values[0], values[1], values[2]}, nil
}

func validateManifest(manifest Manifest) error {
	switch {
	case manifest.Scene == "":
		return fmt.Errorf("invalid manifest: missing scene")
	case manifest.Source == "":
		return fmt.Errorf("invalid manifest: missing source")
	case manifest.Version != ManifestVersion1 && manifest.Version != ManifestVersion2:
		return fmt.Errorf("invalid manifest: version must be %d or %d", ManifestVersion1, ManifestVersion2)
	}
	if err := validateSceneAssetPath(manifest.Scene); err != nil {
		return err
	}

	seenFileIDs := make(map[int64]struct{}, len(manifest.Objects))
	for i, object := range manifest.Objects {
		if object.FileID <= 0 {
			return fmt.Errorf("invalid manifest: objects[%d].fileID must be > 0", i)
		}
		if _, ok := seenFileIDs[object.FileID]; ok {
			return fmt.Errorf("invalid manifest: duplicate objects.fileID=%d", object.FileID)
		}
		seenFileIDs[object.FileID] = struct{}{}

		if err := validateSize(object.Bounds.Size, fmt.Sprintf("objects[%d].bounds.size", i)); err != nil {
			return err
		}
	}

	seenPaths := make(map[string]struct{}, len(manifest.Prefabs))
	for i, prefab := range manifest.Prefabs {
		if err := validatePrefabAssetPath(prefab.Path, i); err != nil {
			return err
		}
		if _, ok := seenPaths[prefab.Path]; ok {
			return fmt.Errorf("invalid manifest: duplicate prefabs.path=%q", prefab.Path)
		}
		seenPaths[prefab.Path] = struct{}{}

		if err := validateSize(prefab.Bounds.Size, fmt.Sprintf("prefabs[%d].bounds.size", i)); err != nil {
			return err
		}
	}

	if manifest.Version == ManifestVersion2 {
		if len(manifest.Capabilities) == 0 {
			return fmt.Errorf("invalid manifest: version 2 requires capabilities")
		}
		for i, prefab := range manifest.Prefabs {
			if prefab.Spatial == nil || len(prefab.Spatial.OBBs) == 0 {
				return fmt.Errorf("invalid manifest: prefabs[%d].spatial.obbs is required for version 2", i)
			}
			if err := validateSpatial(prefab.Spatial, fmt.Sprintf("prefabs[%d].spatial", i)); err != nil {
				return err
			}
		}
		for i, object := range manifest.Objects {
			if object.Spatial != nil {
				if err := validateSpatial(object.Spatial, fmt.Sprintf("objects[%d].spatial", i)); err != nil {
					return err
				}
			}
		}
		seenSurfaceIDs := make(map[string]struct{}, len(manifest.Surfaces))
		for i, surface := range manifest.Surfaces {
			if strings.TrimSpace(surface.ID) == "" {
				return fmt.Errorf("invalid manifest: surfaces[%d].id is required", i)
			}
			if _, ok := seenSurfaceIDs[surface.ID]; ok {
				return fmt.Errorf("invalid manifest: duplicate surfaces.id=%q", surface.ID)
			}
			seenSurfaceIDs[surface.ID] = struct{}{}
			if surface.Type != "floor" && surface.Type != "wall" && surface.Type != "ceiling" {
				return fmt.Errorf("invalid manifest: surfaces[%d].type must be floor|wall|ceiling", i)
			}
			if surface.Size[0] <= 0 || surface.Size[1] <= 0 {
				return fmt.Errorf("invalid manifest: surfaces[%d].size values must be > 0", i)
			}
			if !unitVec3(surface.Normal) || !unitVec3(surface.Tangent) || math.Abs(dotVec3(surface.Normal, surface.Tangent)) > .001 {
				return fmt.Errorf("invalid manifest: surfaces[%d] normal/tangent must be normalized and orthogonal", i)
			}
		}
	}

	return nil
}

func Decode(data []byte) (Manifest, error) { return decodeManifest(data) }

func validateSpatial(profile *SpatialProfile, path string) error {
	if profile.Confidence < 0 || profile.Confidence > 1 {
		return fmt.Errorf("invalid manifest: %s.confidence must be between 0 and 1", path)
	}
	if !unitVec3(profile.Forward) || !unitVec3(profile.Up) || math.Abs(dotVec3(profile.Forward, profile.Up)) > .001 {
		return fmt.Errorf("invalid manifest: %s forward/up must be normalized and orthogonal", path)
	}
	seenOBBIDs := make(map[string]struct{}, len(profile.OBBs))
	for i, box := range profile.OBBs {
		if strings.TrimSpace(box.ID) == "" {
			return fmt.Errorf("invalid manifest: %s.obbs[%d].id is required", path, i)
		}
		if _, exists := seenOBBIDs[box.ID]; exists {
			return fmt.Errorf("invalid manifest: duplicate %s.obbs.id=%q", path, box.ID)
		}
		seenOBBIDs[box.ID] = struct{}{}
		if err := validateSize(box.Size, fmt.Sprintf("%s.obbs[%d].size", path, i)); err != nil {
			return err
		}
		length := box.Rotation[0]*box.Rotation[0] + box.Rotation[1]*box.Rotation[1] + box.Rotation[2]*box.Rotation[2] + box.Rotation[3]*box.Rotation[3]
		if length < 0.999 || length > 1.001 {
			return fmt.Errorf("invalid manifest: %s.obbs[%d].rotation must be normalized", path, i)
		}
	}
	frames := make(map[string]struct{}, len(profile.Frames)+3)
	for index := range profile.Frames {
		frame := &profile.Frames[index]
		if err := validateContactFrame(frame, fmt.Sprintf("%s.frames[%d]", path, index)); err != nil {
			return err
		}
		if _, exists := frames[frame.ID]; exists {
			return fmt.Errorf("invalid manifest: duplicate %s.frames.id=%q", path, frame.ID)
		}
		frames[frame.ID] = struct{}{}
	}
	legacyFrameIDs := make(map[string]string, 3)
	for _, item := range []struct {
		name  string
		frame *ContactFrame
	}{{"bottom_contact", profile.BottomContact}, {"back_contact", profile.BackContact}, {"top_contact", profile.TopContact}} {
		name, frame := item.name, item.frame
		if frame == nil {
			continue
		}
		if err := validateContactFrame(frame, path+"."+name); err != nil {
			return err
		}
		if previous, exists := legacyFrameIDs[frame.ID]; exists {
			return fmt.Errorf("invalid manifest: %s.%s duplicates legacy frame id=%q from %s", path, name, frame.ID, previous)
		}
		legacyFrameIDs[frame.ID] = name
		// A legacy alias may repeat the canonical frame with the same ID and
		// value, but may never silently redefine it.
		if existing := findFrameByID(profile.Frames, frame.ID); existing != nil && *existing != *frame {
			return fmt.Errorf("invalid manifest: %s.%s conflicts with frames id=%q", path, name, frame.ID)
		}
		frames[frame.ID] = struct{}{}
	}
	seenContacts := make(map[string]struct{}, len(profile.Contacts))
	for index, contact := range profile.Contacts {
		if strings.TrimSpace(contact.ID) == "" {
			return fmt.Errorf("invalid manifest: %s.contacts[%d].id is required", path, index)
		}
		if _, exists := seenContacts[contact.ID]; exists {
			return fmt.Errorf("invalid manifest: duplicate %s.contacts.id=%q", path, contact.ID)
		}
		seenContacts[contact.ID] = struct{}{}
		if _, exists := frames[contact.FrameID]; !exists {
			return fmt.Errorf("invalid manifest: %s.contacts[%d] references missing frame %q", path, index, contact.FrameID)
		}
		if contact.Kind != "FloorSupported" && contact.Kind != "WallBacked" && contact.Kind != "WallMounted" && contact.Kind != "CeilingMounted" {
			return fmt.Errorf("invalid manifest: %s.contacts[%d].kind is unsupported", path, index)
		}
		if strings.TrimSpace(contact.Target) == "" || !finite(contact.MinimumGap) || !finite(contact.MaximumGap) || !finite(contact.MaximumPenetration) || !finite(contact.MinimumSupport) || !finite(contact.DirectionAlignment) || contact.MinimumGap < 0 || contact.MaximumGap < contact.MinimumGap || contact.MaximumPenetration < 0 || contact.MinimumSupport < 0 || contact.MinimumSupport > 1 || contact.DirectionAlignment < 0 || contact.DirectionAlignment > 1 {
			return fmt.Errorf("invalid manifest: %s.contacts[%d] has invalid target or tolerances", path, index)
		}
	}
	return nil
}

func validateContactFrame(frame *ContactFrame, path string) error {
	if strings.TrimSpace(frame.ID) == "" || frame.Size[0] <= 0 || frame.Size[1] <= 0 || !unitVec3(frame.Normal) || !unitVec3(frame.Tangent) || math.Abs(dotVec3(frame.Normal, frame.Tangent)) > .001 {
		return fmt.Errorf("invalid manifest: %s has invalid id, basis, or size", path)
	}
	return nil
}

func findFrameByID(frames []ContactFrame, id string) *ContactFrame {
	for index := range frames {
		if frames[index].ID == id {
			return &frames[index]
		}
	}
	return nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func dotVec3(a, b Vec3) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func unitVec3(value Vec3) bool {
	length := dotVec3(value, value)
	return finite(length) && math.Abs(length-1) <= .001
}

func validateSize(size Vec3, path string) error {
	for i, value := range size {
		if value <= 0 {
			return fmt.Errorf("invalid manifest: %s[%d] must be > 0", path, i)
		}
	}

	return nil
}

func validateSceneAssetPath(path string) error {
	path = strings.TrimSpace(path)
	switch {
	case path == "":
		return fmt.Errorf("invalid manifest: scene must be an Assets path ending in .unity")
	case !strings.HasPrefix(path, "Assets/"):
		return fmt.Errorf("invalid manifest: scene must be an Assets path ending in .unity")
	case !strings.HasSuffix(path, ".unity"):
		return fmt.Errorf("invalid manifest: scene must be an Assets path ending in .unity")
	default:
		return nil
	}
}

func validatePrefabAssetPath(path string, index int) error {
	path = strings.TrimSpace(path)
	switch {
	case path == "":
		return fmt.Errorf("invalid manifest: prefabs[%d].path %s", index, gameObjectAssetPathRequirement)
	case !strings.HasPrefix(path, "Assets/"):
		return fmt.Errorf("invalid manifest: prefabs[%d].path %s", index, gameObjectAssetPathRequirement)
	case !isSupportedGameObjectAssetPath(path):
		return fmt.Errorf("invalid manifest: prefabs[%d].path %s", index, gameObjectAssetPathRequirement)
	default:
		return nil
	}
}

func isSupportedGameObjectAssetPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".prefab", ".fbx", ".dae", ".3ds", ".dxf", ".obj", ".skp", ".blend", ".max", ".ma", ".mb":
		return true
	default:
		return false
	}
}
