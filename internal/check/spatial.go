package check

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
)

var ErrNeedGeometryV2 = errors.New("NEED_GEOMETRY_V2")

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
	rotation, err := normalizedQuat(req.Rotation)
	if err != nil {
		return SpatialResult{}, err
	}
	placed := transformProfile(prefab.Spatial, req.Position, rotation)
	result := SpatialResult{Clear: true}
	for _, object := range req.Manifest.Objects {
		var objectBoxes []worldOBB
		if object.Spatial != nil && len(object.Spatial.OBBs) > 0 {
			objectBoxes = transformProfile(object.Spatial, bounds.Vec3{}, bounds.Quat{0, 0, 0, 1})
		} else {
			objectBoxes = []worldOBB{aabbOBB(object.Bounds)}
		}
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
	var frame *bounds.ContactFrame
	minGap, maxGap := 0.0, 0.01
	switch req.Contact {
	case "wall-backed":
		frame = prefab.Spatial.BackContact
		minGap, maxGap = 0.01, 0.05
	case "wall-mounted":
		frame = prefab.Spatial.BackContact
		minGap, maxGap = 0.005, 0.01
	case "floor-supported", "ceiling-mounted":
		frame = prefab.Spatial.BottomContact
	default:
		result.Codes = append(result.Codes, "CONTACT_DIRECTION")
		return
	}
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
	if result.Penetration > 1e-6 {
		result.Codes = append(result.Codes, "SURFACE_PENETRATION")
	}
	if result.Gap < minGap-1e-6 || result.Gap > maxGap+1e-6 {
		result.Codes = append(result.Codes, "CONTACT_GAP")
	}
	if result.Alignment < 0.95 {
		result.Codes = append(result.Codes, "CONTACT_DIRECTION")
	}
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
func aabbOBB(box bounds.AABB) worldOBB {
	return worldOBB{center: box.Center, extents: mul(box.Size, .5), axes: [3]bounds.Vec3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}
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
