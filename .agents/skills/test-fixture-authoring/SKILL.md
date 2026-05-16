---
name: test-fixture-authoring
description: Use when creating or modifying testdata Unity YAML fixtures.
---

# Test Fixture Authoring Skill

## Goals

Test parser and command behavior without requiring Unity Editor.

## Fixture Rules

- Keep fixtures small.
- Include only YAML blocks required for the test.
- Use stable fake fileIDs.
- Use realistic Unity classID headers.
- Include comments outside YAML blocks if needed, but do not rely on comments in parser behavior.
- Do not modify large real project files for unit tests.

## Required Fixtures

- simple_scene.unity
- duplicate_names_scene.unity
- nested_prefab_scene.unity
- enemy.prefab
- enemy_config.asset
- material.mat
- unknown_component.prefab

## Tests Must Cover

- unknown classID preservation
- name ambiguity
- fileID lookup
- field get
- field not found
- compact output stability
