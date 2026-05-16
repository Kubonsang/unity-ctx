---
name: unity-yaml-parser
description: Use when working on Unity YAML parsing, object blocks, classID mapping, fileID relationships, or serialized field access.
---

# Unity YAML Parser Skill

## Goals

Parse Unity YAML enough for context extraction and safe inspection.

## Required Capabilities

- Split documents by `--- !u!<classID> &<fileID>`
- Preserve fileID as stable identity
- Map common classIDs:
  - 1 GameObject
  - 4 Transform
  - 20 Camera
  - 23 MeshRenderer
  - 33 MeshFilter
  - 54 Rigidbody
  - 65 BoxCollider
  - 114 MonoBehaviour
  - 1001 PrefabInstance
- Preserve unknown classIDs as `UNKNOWN_COMPONENT`
- Support scalar, bool, int, float, Vector2, Vector3, Vector4, Color-like sequences
- Support dot notation for nested fields

## Rules

- Do not discard unknown blocks.
- Do not guess MonoBehaviour script types unless GUID/script info is available.
- If a field cannot be resolved, return `FIELD_NOT_FOUND`.
- If multiple objects match a name, return `AMBIGUOUS_NAME`.
- Parser output must be deterministic.
