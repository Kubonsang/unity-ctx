# Testing Guide

## Required Command

```bash
go test ./...
```

## Unit Tests Must Not Require Unity Editor

Use small fixtures under `testdata/`.

## Required Fixtures

- `testdata/scenes/simple_scene.unity`
- `testdata/scenes/duplicate_names_scene.unity`
- `testdata/prefabs/enemy.prefab`
- `testdata/assets/enemy_config.asset`
- `testdata/assets/material.mat`
- `testdata/prefabs/unknown_component.prefab`

## Test Categories

### Parser

- split Unity YAML document blocks
- preserve classID and fileID
- preserve unknown classIDs
- parse scalar values
- parse vector-like values
- resolve dot notation

### Commands

- summarize success
- query by id
- query by name
- ambiguous name
- inspect component
- get field
- field not found
- invalid args
- missing file

### Output

- deterministic ordering
- stable compact output
- correct exit codes
