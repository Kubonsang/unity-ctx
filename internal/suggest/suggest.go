package suggest

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
	"github.com/Kubonsang/unity-ctx/internal/check"
)

type Align string

const (
	AlignFloor Align = "floor"
	AlignGrid  Align = "grid"
	AlignWall  Align = "wall"
)

type Anchor struct {
	FileID int64
	Name   string
}

type Candidate struct {
	Rank       int
	Status     string
	Direction  string
	Position   bounds.Vec3
	OverlapIDs []int64
	Rotation   bounds.Quat
}

type Request struct {
	Manifest  bounds.Manifest
	Prefab    string
	Near      string
	Count     int
	Align     Align
	SurfaceID string
	Contact   string
}

type Result struct {
	Status     string
	Manifest   string
	PrefabPath string
	Near       Anchor
	Align      Align
	Contact    string
	Count      int
	Candidates []Candidate
}

type directionSpec struct {
	name  string
	order int
}

var directionOrder = []directionSpec{
	{name: "east", order: 0},
	{name: "west", order: 1},
	{name: "north", order: 2},
	{name: "south", order: 3},
}

func Plan(req Request) (Result, error) {
	if req.Count < 1 {
		return Result{}, fmt.Errorf("count must be >= 1")
	}
	align := req.Align
	if align == "" {
		align = AlignFloor
	}
	if align == AlignWall {
		return planWall(req)
	}

	anchorObject, err := resolveAnchor(req.Manifest, req.Near)
	if err != nil {
		return Result{}, err
	}

	anchor := Anchor{
		FileID: anchorObject.FileID,
		Name:   anchorObject.Name,
	}

	prefabBounds, err := prefabBounds(req.Manifest, req.Prefab)
	if err != nil {
		return Result{}, err
	}

	candidates := make([]Candidate, 0, len(directionOrder))
	for _, spec := range directionOrder {
		position := candidatePosition(anchorObject.Bounds, prefabBounds, spec.name, align)
		placement, err := check.CheckPlacement(req.Manifest, req.Prefab, position)
		if err != nil {
			return Result{}, err
		}
		overlapIDs := excludeFileID(placement.OverlapIDs, anchorObject.FileID)

		status := "WARN"
		if len(overlapIDs) == 0 {
			status = "OK"
		}

		candidates = append(candidates, Candidate{
			Status:     status,
			Direction:  spec.name,
			Position:   position,
			OverlapIDs: overlapIDs,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]

		if left.Status != right.Status {
			return statusRank(left.Status) < statusRank(right.Status)
		}

		leftDir := directionRank(left.Direction)
		rightDir := directionRank(right.Direction)
		if leftDir != rightDir {
			return leftDir < rightDir
		}

		if left.Position[0] != right.Position[0] {
			return left.Position[0] < right.Position[0]
		}
		if left.Position[1] != right.Position[1] {
			return left.Position[1] < right.Position[1]
		}
		return left.Position[2] < right.Position[2]
	})

	limit := req.Count
	if limit > len(candidates) {
		limit = len(candidates)
	}
	if limit > 4 {
		limit = 4
	}
	candidates = append([]Candidate(nil), candidates[:limit]...)
	for i := range candidates {
		candidates[i].Rank = i + 1
	}

	result := Result{
		Status:     "WARN",
		Manifest:   req.Manifest.Scene,
		PrefabPath: req.Prefab,
		Near:       anchor,
		Align:      align,
		Count:      limit,
		Candidates: candidates,
	}
	for _, candidate := range candidates {
		if candidate.Status == "OK" {
			result.Status = "OK"
			break
		}
	}

	return result, nil
}

func planWall(req Request) (Result, error) {
	if req.Manifest.Version != bounds.ManifestVersion2 {
		return Result{}, check.ErrNeedGeometryV2
	}
	var surface *bounds.SurfacePatch
	for i := range req.Manifest.Surfaces {
		if req.Manifest.Surfaces[i].ID == req.SurfaceID {
			surface = &req.Manifest.Surfaces[i]
			break
		}
	}
	if surface == nil {
		return Result{}, fmt.Errorf("missing surface id=%q", req.SurfaceID)
	}
	if !surface.Supported || !surface.Reviewed {
		return Result{}, fmt.Errorf("SURFACE_UNREVIEWED id=%q", req.SurfaceID)
	}
	prefab, ok := findPrefabEntry(req.Manifest.Prefabs, req.Prefab)
	if !ok || prefab.Spatial == nil || len(prefab.Spatial.OBBs) == 0 {
		return Result{}, check.ErrNeedGeometryV2
	}
	if !prefab.Spatial.Reviewed {
		return Result{}, check.ErrGeometryUnreviewed
	}
	wallRequirement, err := resolveWallRequirement(prefab.Spatial.Contacts, req.Contact)
	if err != nil {
		return Result{}, err
	}
	wallFrame := profileFrame(prefab.Spatial, wallRequirement.FrameID)
	if wallFrame == nil {
		return Result{}, check.ErrGeometryUnreviewed
	}
	rotation := lookRotation(surface.Normal, cross(surface.Normal, surface.Tangent))
	gap := (wallRequirement.MinimumGap + wallRequirement.MaximumGap) * 0.5
	offsets := []float64{-0.3, -0.1, 0.1, 0.3}
	candidates := make([]Candidate, 0, 4)
	for index, factor := range offsets {
		point := add(surface.Origin, mul(normalize(surface.Tangent), surface.Size[0]*factor))
		point = add(point, mul(normalize(surface.Normal), gap))
		contactOffset := rotate(rotation, wallFrame.Point)
		position := sub(point, contactOffset)
		var floorChecked *check.SpatialResult
		if floorRequirement := findRequirement(prefab.Spatial.Contacts, "floor-supported"); floorRequirement != nil {
			var floorResult check.SpatialResult
			position, floorResult, err = projectToRequiredFloor(req.Manifest, req.Prefab, prefab.Spatial, *floorRequirement, position, rotation)
			if err != nil {
				return Result{}, err
			}
			floorChecked = &floorResult
		}
		checked, err := check.CheckSpatialPlacement(check.SpatialRequest{Manifest: req.Manifest, Prefab: req.Prefab, Position: position, Rotation: rotation, SurfaceID: req.SurfaceID, Contact: canonicalContact(wallRequirement.Kind)})
		if err != nil {
			return Result{}, err
		}
		status := "OK"
		if !checked.Clear || (floorChecked != nil && !floorChecked.Clear) {
			status = "WARN"
		}
		overlaps := append([]int64(nil), checked.OverlapIDs...)
		if floorChecked != nil {
			overlaps = append(overlaps, floorChecked.OverlapIDs...)
		}
		sort.Slice(overlaps, func(i, j int) bool { return overlaps[i] < overlaps[j] })
		overlaps = uniqueIDs(overlaps)
		candidates = append(candidates, Candidate{Rank: index + 1, Status: status, Direction: fmt.Sprintf("wall-%d", index+1), Position: position, Rotation: rotation, OverlapIDs: overlaps})
	}
	limit := req.Count
	if limit < 1 || limit > 4 {
		limit = 4
	}
	candidates = candidates[:limit]
	status := "WARN"
	for _, candidate := range candidates {
		if candidate.Status == "OK" {
			status = "OK"
			break
		}
	}
	return Result{Status: status, Manifest: req.Manifest.Scene, PrefabPath: req.Prefab, Near: Anchor{Name: req.SurfaceID}, Align: AlignWall, Contact: canonicalContact(wallRequirement.Kind), Count: len(candidates), Candidates: candidates}, nil
}

func resolveWallRequirement(items []bounds.ContactRequirement, requested string) (bounds.ContactRequirement, error) {
	requested = canonicalContact(requested)
	matches := make([]bounds.ContactRequirement, 0, 2)
	for _, item := range items {
		kind := canonicalContact(item.Kind)
		if kind != "wall-backed" && kind != "wall-mounted" {
			continue
		}
		if requested == "" || requested == kind {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return bounds.ContactRequirement{}, fmt.Errorf("SUPPORT_CONTRACT_MISSING contact=%q", requested)
	case 1:
		return matches[0], nil
	default:
		return bounds.ContactRequirement{}, fmt.Errorf("CONTACT_REQUIREMENT_AMBIGUOUS: specify wall-backed or wall-mounted")
	}
}

func findRequirement(items []bounds.ContactRequirement, requested string) *bounds.ContactRequirement {
	requested = canonicalContact(requested)
	for index := range items {
		if canonicalContact(items[index].Kind) == requested {
			return &items[index]
		}
	}
	return nil
}

func canonicalContact(value string) string {
	switch strings.TrimSpace(value) {
	case "WallBacked", "wall-backed":
		return "wall-backed"
	case "WallMounted", "wall-mounted":
		return "wall-mounted"
	case "FloorSupported", "floor-supported":
		return "floor-supported"
	case "CeilingMounted", "ceiling-mounted":
		return "ceiling-mounted"
	default:
		return ""
	}
}

func profileFrame(profile *bounds.SpatialProfile, id string) *bounds.ContactFrame {
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

func projectToRequiredFloor(manifest bounds.Manifest, prefabPath string, profile *bounds.SpatialProfile, requirement bounds.ContactRequirement, position bounds.Vec3, rotation bounds.Quat) (bounds.Vec3, check.SpatialResult, error) {
	frame := profileFrame(profile, requirement.FrameID)
	if frame == nil {
		return bounds.Vec3{}, check.SpatialResult{}, check.ErrGeometryUnreviewed
	}
	surfaces := append([]bounds.SurfacePatch(nil), manifest.Surfaces...)
	sort.Slice(surfaces, func(i, j int) bool { return surfaces[i].ID < surfaces[j].ID })
	for _, surface := range surfaces {
		if surface.Type != "floor" || !surface.Reviewed || !surface.Supported {
			continue
		}
		point := add(position, rotate(rotation, frame.Point))
		normal := normalize(surface.Normal)
		currentGap := dot(sub(point, surface.Origin), normal)
		targetGap := (requirement.MinimumGap + requirement.MaximumGap) * 0.5
		candidate := add(position, mul(normal, targetGap-currentGap))
		checked, err := check.CheckSpatialPlacement(check.SpatialRequest{Manifest: manifest, Prefab: prefabPath, Position: candidate, Rotation: rotation, SurfaceID: surface.ID, Contact: "floor-supported"})
		if err != nil {
			return bounds.Vec3{}, check.SpatialResult{}, err
		}
		if onlyOverlapCodes(checked.Codes) {
			return candidate, checked, nil
		}
	}
	return bounds.Vec3{}, check.SpatialResult{}, fmt.Errorf("SUPPORT_REGION_INVALID: no reviewed floor can satisfy %s", requirement.ID)
}

func onlyOverlapCodes(codes []string) bool {
	for _, code := range codes {
		if code != "OBB_OVERLAP" {
			return false
		}
	}
	return true
}

func uniqueIDs(values []int64) []int64 {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func findPrefabEntry(items []bounds.PrefabBounds, path string) (bounds.PrefabBounds, bool) {
	for _, item := range items {
		if item.Path == path {
			return item, true
		}
	}
	return bounds.PrefabBounds{}, false
}
func add(a, b bounds.Vec3) bounds.Vec3         { return bounds.Vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func sub(a, b bounds.Vec3) bounds.Vec3         { return bounds.Vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func mul(a bounds.Vec3, s float64) bounds.Vec3 { return bounds.Vec3{a[0] * s, a[1] * s, a[2] * s} }
func dot(a, b bounds.Vec3) float64             { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func cross(a, b bounds.Vec3) bounds.Vec3 {
	return bounds.Vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func normalize(v bounds.Vec3) bounds.Vec3 {
	length := math.Sqrt(dot(v, v))
	if length < 1e-12 {
		return bounds.Vec3{}
	}
	return mul(v, 1/length)
}
func rotate(q bounds.Quat, v bounds.Vec3) bounds.Vec3 {
	u := bounds.Vec3{q[0], q[1], q[2]}
	s := q[3]
	return add(add(mul(u, 2*dot(u, v)), mul(v, s*s-dot(u, u))), mul(cross(u, v), 2*s))
}
func lookRotation(forward, up bounds.Vec3) bounds.Quat {
	f := normalize(forward)
	r := normalize(cross(up, f))
	u := cross(f, r)
	m00, m11, m22 := r[0], u[1], f[2]
	trace := m00 + m11 + m22
	var q bounds.Quat
	if trace > 0 {
		s := math.Sqrt(trace+1) * 2
		q = bounds.Quat{(u[2] - f[1]) / s, (f[0] - r[2]) / s, (r[1] - u[0]) / s, s / 4}
	} else if m00 > m11 && m00 > m22 {
		s := math.Sqrt(1+m00-m11-m22) * 2
		q = bounds.Quat{s / 4, (r[1] + u[0]) / s, (f[0] + r[2]) / s, (u[2] - f[1]) / s}
	} else if m11 > m22 {
		s := math.Sqrt(1+m11-m00-m22) * 2
		q = bounds.Quat{(r[1] + u[0]) / s, s / 4, (u[2] + f[1]) / s, (f[0] - r[2]) / s}
	} else {
		s := math.Sqrt(1+m22-m00-m11) * 2
		q = bounds.Quat{(f[0] + r[2]) / s, (u[2] + f[1]) / s, s / 4, (r[1] - u[0]) / s}
	}
	return q
}

func resolveAnchor(manifest bounds.Manifest, near string) (bounds.ObjectBounds, error) {
	trimmed := strings.TrimSpace(near)
	if trimmed == "" {
		return bounds.ObjectBounds{}, fmt.Errorf(`missing anchor near=""`)
	}

	if id, ok := parseNonZeroDecimal(trimmed); ok {
		for _, object := range manifest.Objects {
			if object.FileID == id {
				return object, nil
			}
		}
		return bounds.ObjectBounds{}, fmt.Errorf("missing anchor near=%q", trimmed)
	}

	matches := make([]bounds.ObjectBounds, 0, 1)
	for _, object := range manifest.Objects {
		if object.Name == trimmed {
			matches = append(matches, object)
		}
	}

	switch len(matches) {
	case 0:
		return bounds.ObjectBounds{}, fmt.Errorf("missing anchor near=%q", trimmed)
	case 1:
		return matches[0], nil
	default:
		return bounds.ObjectBounds{}, fmt.Errorf("AMBIGUOUS_NAME name=%q matches=%d", trimmed, len(matches))
	}
}

func prefabBounds(manifest bounds.Manifest, prefabPath string) (bounds.AABB, error) {
	for _, prefab := range manifest.Prefabs {
		if prefab.Path == prefabPath {
			return prefab.Bounds, nil
		}
	}

	_, err := check.CheckPlacement(manifest, prefabPath, bounds.Vec3{})
	if err != nil {
		return bounds.AABB{}, err
	}
	return bounds.AABB{}, fmt.Errorf("missing prefab manifest entry for path=%q", prefabPath)
}

func candidatePosition(anchor bounds.AABB, prefab bounds.AABB, direction string, align Align) bounds.Vec3 {
	center := bounds.Vec3{
		anchor.Center[0],
		prefab.Size[1] / 2,
		anchor.Center[2],
	}

	switch direction {
	case "east":
		center[0] += anchor.Size[0]/2 + prefab.Size[0]/2
	case "west":
		center[0] -= anchor.Size[0]/2 + prefab.Size[0]/2
	case "north":
		center[2] += anchor.Size[2]/2 + prefab.Size[2]/2
	case "south":
		center[2] -= anchor.Size[2]/2 + prefab.Size[2]/2
	}

	if align == AlignGrid {
		center[0] = snapHalfMeter(center[0])
		center[2] = snapHalfMeter(center[2])
	}

	return bounds.Vec3{
		center[0] - prefab.Center[0],
		center[1] - prefab.Center[1],
		center[2] - prefab.Center[2],
	}
}

func snapHalfMeter(value float64) float64 {
	return math.Round(value*2) / 2
}

func parseNonZeroDecimal(raw string) (int64, bool) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value == 0 {
		return 0, false
	}
	return value, true
}

func excludeFileID(ids []int64, fileID int64) []int64 {
	if len(ids) == 0 {
		return nil
	}

	filtered := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == fileID {
			continue
		}
		filtered = append(filtered, id)
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func statusRank(status string) int {
	switch status {
	case "OK":
		return 0
	case "WARN":
		return 1
	default:
		return 2
	}
}

func directionRank(direction string) int {
	for _, spec := range directionOrder {
		if spec.name == direction {
			return spec.order
		}
	}
	return len(directionOrder)
}
