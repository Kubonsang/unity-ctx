package check

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
)

var (
	ErrNeedGeometryV2         = errors.New("NEED_GEOMETRY_V2")
	ErrGeometryUnreviewed     = errors.New("GEOMETRY_UNREVIEWED")
	ErrRoomGeometryUnreviewed = errors.New("ROOM_GEOMETRY_UNREVIEWED")
)

type SpatialRequest struct {
	Manifest  bounds.Manifest
	Prefab    string
	Position  bounds.Vec3
	Rotation  bounds.Quat
	SurfaceID string
	Contact   string
}

type SpatialResult struct {
	Clear       bool
	OverlapIDs  []int64
	Codes       []string
	Gap         float64
	Penetration float64
	Alignment   float64
	Support     float64
}

type worldOBB struct {
	center  bounds.Vec3
	extents bounds.Vec3
	axes    [3]bounds.Vec3
}

func CheckSpatialPlacement(req SpatialRequest) (SpatialResult, error) {
	if req.Manifest.Version != bounds.ManifestVersion2 {
		return SpatialResult{}, ErrNeedGeometryV2
	}
	prefab, ok := findPrefab(req.Manifest.Prefabs, req.Prefab)
	if !ok {
		return SpatialResult{}, fmt.Errorf("missing prefab manifest entry for path=%q", req.Prefab)
	}
	if prefab.Spatial == nil || len(prefab.Spatial.OBBs) == 0 {
		return SpatialResult{}, ErrNeedGeometryV2
	}
	if !prefab.Spatial.Reviewed {
		return SpatialResult{}, ErrGeometryUnreviewed
	}
	rotation, err := normalizedQuat(req.Rotation)
	if err != nil {
		return SpatialResult{}, err
	}
	placed := transformProfile(prefab.Spatial, req.Position, rotation)
	result := SpatialResult{Clear: true}
	for _, object := range req.Manifest.Objects {
		if object.Spatial == nil || len(object.Spatial.OBBs) == 0 {
			return SpatialResult{}, ErrRoomGeometryUnreviewed
		}
		if !object.Spatial.Reviewed {
			return SpatialResult{}, ErrRoomGeometryUnreviewed
		}
		objectBoxes := transformProfile(object.Spatial, bounds.Vec3{}, bounds.Quat{0, 0, 0, 1})
		if intersectsAny(placed, objectBoxes) {
			result.OverlapIDs = append(result.OverlapIDs, object.FileID)
		}
	}
	if len(result.OverlapIDs) > 0 {
		result.Codes = append(result.Codes, "OBB_OVERLAP")
	}
	sort.Slice(result.OverlapIDs, func(i, j int) bool { return result.OverlapIDs[i] < result.OverlapIDs[j] })
	if req.Contact != "" || req.SurfaceID != "" {
		if req.Contact == "" || req.SurfaceID == "" {
			return SpatialResult{}, fmt.Errorf("surface-id and contact must be provided together")
		}
		evaluateContact(req, prefab, rotation, &result)
	}
	result.Clear = len(result.Codes) == 0
	return result, nil
}

func evaluateContact(req SpatialRequest, prefab bounds.PrefabBounds, rotation bounds.Quat, result *SpatialResult) {
	var surface *bounds.SurfacePatch
	for i := range req.Manifest.Surfaces {
		if req.Manifest.Surfaces[i].ID == req.SurfaceID {
			surface = &req.Manifest.Surfaces[i]
			break
		}
	}
	if surface == nil {
		result.Codes = append(result.Codes, "SURFACE_UNREVIEWED")
		return
	}
	if !surface.Supported {
		result.Codes = append(result.Codes, "UNSUPPORTED_SURFACE")
		return
	}
	if !surface.Reviewed {
		result.Codes = append(result.Codes, "SURFACE_UNREVIEWED")
	}
	contact := canonicalContactKind(req.Contact)
	wantSurfaceType := surfaceTypeForContact(contact)
	if wantSurfaceType == "" || surface.Type != wantSurfaceType {
		result.Codes = append(result.Codes, "CONTACT_DIRECTION")
		return
	}
	requirement := findContactRequirement(prefab.Spatial.Contacts, contact)
	if requirement == nil {
		result.Codes = append(result.Codes, "SUPPORT_CONTRACT_MISSING")
		return
	}
	frame := findContactFrame(prefab.Spatial, requirement.FrameID)
	if frame == nil {
		result.Codes = append(result.Codes, "GEOMETRY_UNREVIEWED")
		return
	}
	point := add(req.Position, rotate(rotation, frame.Point))
	normal := normalize(rotate(rotation, frame.Normal))
	surfaceNormal := normalize(surface.Normal)
	signed := dot(sub(point, surface.Origin), surfaceNormal)
	result.Gap = math.Max(0, signed)
	result.Penetration = math.Max(0, -signed)
	result.Alignment = dot(normal, mul(surfaceNormal, -1))
	if result.Penetration > requirement.MaximumPenetration+1e-6 {
		result.Codes = append(result.Codes, "SURFACE_PENETRATION")
	}
	if result.Gap < requirement.MinimumGap-1e-6 || result.Gap > requirement.MaximumGap+1e-6 {
		result.Codes = append(result.Codes, "CONTACT_GAP")
	}
	if result.Alignment < requirement.DirectionAlignment {
		result.Codes = append(result.Codes, "CONTACT_DIRECTION")
	}
	result.Support = contactSupport(*surface, *frame, req.Position, rotation)
	if result.Support+1e-6 < requirement.MinimumSupport {
		result.Codes = append(result.Codes, "INSUFFICIENT_SUPPORT")
	}
}

func canonicalContactKind(value string) string {
	switch value {
	case "wall-backed", "WallBacked":
		return "wall-backed"
	case "wall-mounted", "WallMounted":
		return "wall-mounted"
	case "floor-supported", "FloorSupported":
		return "floor-supported"
	case "ceiling-mounted", "CeilingMounted":
		return "ceiling-mounted"
	default:
		return ""
	}
}

func surfaceTypeForContact(contact string) string {
	switch contact {
	case "wall-backed", "wall-mounted":
		return "wall"
	case "floor-supported":
		return "floor"
	case "ceiling-mounted":
		return "ceiling"
	default:
		return ""
	}
}

func findContactRequirement(items []bounds.ContactRequirement, contact string) *bounds.ContactRequirement {
	for index := range items {
		if canonicalContactKind(items[index].Kind) == contact {
			return &items[index]
		}
	}
	return nil
}

func findContactFrame(profile *bounds.SpatialProfile, id string) *bounds.ContactFrame {
	if profile.BottomContact != nil && profile.BottomContact.ID == id {
		return profile.BottomContact
	}
	if profile.BackContact != nil && profile.BackContact.ID == id {
		return profile.BackContact
	}
	if profile.TopContact != nil && profile.TopContact.ID == id {
		return profile.TopContact
	}
	return nil
}

type point2 struct{ x, y float64 }

func contactSupport(surface bounds.SurfacePatch, frame bounds.ContactFrame, position bounds.Vec3, rotation bounds.Quat) float64 {
	surfaceTangent := normalize(surface.Tangent)
	surfaceBitangent := normalize(cross(surface.Normal, surfaceTangent))
	framePoint := add(position, rotate(rotation, frame.Point))
	frameNormal := normalize(rotate(rotation, frame.Normal))
	frameTangent := normalize(rotate(rotation, frame.Tangent))
	frameBitangent := normalize(cross(frameNormal, frameTangent))
	halfX, halfY := frame.Size[0]*0.5, frame.Size[1]*0.5
	polygon := make([]point2, 0, 4)
	for _, signs := range [][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		corner := add(framePoint, add(mul(frameTangent, signs[0]*halfX), mul(frameBitangent, signs[1]*halfY)))
		delta := sub(corner, surface.Origin)
		polygon = append(polygon, point2{dot(delta, surfaceTangent), dot(delta, surfaceBitangent)})
	}
	area := polygonArea(polygon)
	if area <= 1e-12 {
		return 0
	}
	clipped := clipAxis(polygon, 0, -surface.Size[0]*0.5, true)
	clipped = clipAxis(clipped, 0, surface.Size[0]*0.5, false)
	clipped = clipAxis(clipped, 1, -surface.Size[1]*0.5, true)
	clipped = clipAxis(clipped, 1, surface.Size[1]*0.5, false)
	ratio := polygonArea(clipped) / area
	return math.Max(0, math.Min(1, ratio))
}

func clipAxis(points []point2, axis int, limit float64, keepGreater bool) []point2 {
	if len(points) == 0 {
		return nil
	}
	inside := func(point point2) bool {
		value := point.x
		if axis == 1 {
			value = point.y
		}
		if keepGreater {
			return value >= limit-1e-12
		}
		return value <= limit+1e-12
	}
	valueAt := func(point point2) float64 {
		if axis == 1 {
			return point.y
		}
		return point.x
	}
	result := make([]point2, 0, len(points)+2)
	previous := points[len(points)-1]
	previousInside := inside(previous)
	for _, current := range points {
		currentInside := inside(current)
		if currentInside != previousInside {
			denominator := valueAt(current) - valueAt(previous)
			if math.Abs(denominator) > 1e-12 {
				t := (limit - valueAt(previous)) / denominator
				result = append(result, point2{previous.x + (current.x-previous.x)*t, previous.y + (current.y-previous.y)*t})
			}
		}
		if currentInside {
			result = append(result, current)
		}
		previous, previousInside = current, currentInside
	}
	return result
}

func polygonArea(points []point2) float64 {
	if len(points) < 3 {
		return 0
	}
	area := 0.0
	for index, point := range points {
		next := points[(index+1)%len(points)]
		area += point.x*next.y - next.x*point.y
	}
	return math.Abs(area) * 0.5
}

func findPrefab(items []bounds.PrefabBounds, path string) (bounds.PrefabBounds, bool) {
	for _, item := range items {
		if item.Path == path {
			return item, true
		}
	}
	return bounds.PrefabBounds{}, false
}
func transformProfile(profile *bounds.SpatialProfile, position bounds.Vec3, rotation bounds.Quat) []worldOBB {
	result := make([]worldOBB, 0, len(profile.OBBs))
	for _, box := range profile.OBBs {
		local, _ := normalizedQuat(box.Rotation)
		combined := quatMul(rotation, local)
		result = append(result, worldOBB{center: add(position, rotate(rotation, box.Center)), extents: mul(box.Size, 0.5), axes: [3]bounds.Vec3{rotate(combined, bounds.Vec3{1, 0, 0}), rotate(combined, bounds.Vec3{0, 1, 0}), rotate(combined, bounds.Vec3{0, 0, 1})}})
	}
	return result
}
func intersectsAny(a, b []worldOBB) bool {
	for _, left := range a {
		for _, right := range b {
			if intersectsOBB(left, right) {
				return true
			}
		}
	}
	return false
}
func intersectsOBB(a, b worldOBB) bool {
	axes := []bounds.Vec3{a.axes[0], a.axes[1], a.axes[2], b.axes[0], b.axes[1], b.axes[2]}
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			axes = append(axes, cross(a.axes[i], b.axes[j]))
		}
	}
	delta := sub(b.center, a.center)
	for _, raw := range axes {
		if dot(raw, raw) < 1e-12 {
			continue
		}
		axis := normalize(raw)
		if math.Abs(dot(delta, axis)) >= radius(a, axis)+radius(b, axis)-1e-6 {
			return false
		}
	}
	return true
}
func radius(box worldOBB, axis bounds.Vec3) float64 {
	return math.Abs(dot(box.axes[0], axis))*box.extents[0] + math.Abs(dot(box.axes[1], axis))*box.extents[1] + math.Abs(dot(box.axes[2], axis))*box.extents[2]
}
func normalizedQuat(q bounds.Quat) (bounds.Quat, error) {
	l := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3])
	if l < 1e-9 {
		return bounds.Quat{}, fmt.Errorf("rotation quaternion must be non-zero")
	}
	if math.Abs(l-1) > 1e-3 {
		return bounds.Quat{}, fmt.Errorf("rotation quaternion must be normalized")
	}
	return bounds.Quat{q[0] / l, q[1] / l, q[2] / l, q[3] / l}, nil
}
func quatMul(a, b bounds.Quat) bounds.Quat {
	return bounds.Quat{a[3]*b[0] + a[0]*b[3] + a[1]*b[2] - a[2]*b[1], a[3]*b[1] - a[0]*b[2] + a[1]*b[3] + a[2]*b[0], a[3]*b[2] + a[0]*b[1] - a[1]*b[0] + a[2]*b[3], a[3]*b[3] - a[0]*b[0] - a[1]*b[1] - a[2]*b[2]}
}
func rotate(q bounds.Quat, v bounds.Vec3) bounds.Vec3 {
	u := bounds.Vec3{q[0], q[1], q[2]}
	s := q[3]
	return add(add(mul(u, 2*dot(u, v)), mul(v, s*s-dot(u, u))), mul(cross(u, v), 2*s))
}
func add(a, b bounds.Vec3) bounds.Vec3         { return bounds.Vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func sub(a, b bounds.Vec3) bounds.Vec3         { return bounds.Vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func mul(a bounds.Vec3, s float64) bounds.Vec3 { return bounds.Vec3{a[0] * s, a[1] * s, a[2] * s} }
func dot(a, b bounds.Vec3) float64             { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func cross(a, b bounds.Vec3) bounds.Vec3 {
	return bounds.Vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func normalize(v bounds.Vec3) bounds.Vec3 {
	l := math.Sqrt(dot(v, v))
	if l < 1e-12 {
		return bounds.Vec3{}
	}
	return mul(v, 1/l)
}
