---
name: use-unity-ctx
description: |
  Unity 씬(.unity), 프리팹(.prefab), 에셋(.asset) 파일을 읽거나 수정해야 할 때 반드시 사용하는 스킬.
  raw YAML 직접 접근(cat, Read, Edit) 대신 unity-ctx CLI를 통해 작업하는 방법과 안전 규칙을 안내한다.
  다음 상황에서 반드시 이 스킬을 먼저 로드할 것:
  - .unity / .prefab / .asset 파일 열기, 조사, 수정 요청
  - 씬 오브젝트 목록 조회 또는 컴포넌트 값 변경
  - 프리팹 필드 설정 또는 씬에 배치
  - Unity 에셋 파일에서 특정 값을 읽거나 패치
---

# use-unity-ctx

Unity 씬/프리팹/에셋을 다룰 때 raw YAML 직접 접근 대신 **unity-ctx CLI**를 사용하는 방법을 안내한다.

## Hard Rules

아래 규칙은 **예외 없이** 적용된다. 위반 시 씬 파일 손상이나 에디터 충돌이 발생할 수 있다.

1. `.unity` / `.prefab` / `.asset` 파일을 직접 `Read`, `cat`, `Edit`하지 않는다. 반드시 unity-ctx 커맨드를 사용한다.
2. 변경 커맨드(`set`, `apply`)는 항상 **dry-run 먼저** 실행하고, 출력을 확인한 후 `--write`를 추가한다.
3. `prefab set --write` 시에는 반드시 `--ack-impact`도 함께 전달한다.
4. GUID를 모를 경우 **추측하지 않는다**. `--prefab-guid` 없이 patch를 작성하면 `UNKNOWN` 상태로 남으며, GUID 확보 전까지 `apply`하지 않는다.
5. 오브젝트 특정은 이름 대신 **fileID**를 사용한다. 이름 사용 시 `WARN` 또는 `ERROR AMBIGUOUS_NAME`이 발생할 수 있다.
6. `scene apply` 전에 반드시 `scene diff`로 patch 내용을 확인한다.
7. Editor 의존성은 커맨드별로 다르다:
   - **Editor 필요**: `scene scan`
   - **Editor 불필요**: `scene list`, `scene inspect`, `scene diff`, `scene patch`, `scene apply`, `asset get/set`, `prefab guid/set`

## 빠른 작업 패턴

### 씬 오브젝트 조사

```bash
# 1. 씬 스캔 — Editor 실행 중에만 동작, scene list/inspect는 Editor 불필요
unity-ctx scene scan --scene Stage01.unity

# 2. 오브젝트 목록 조회 → 여기서 fileID를 확인한다
unity-ctx scene list --scene Stage01.unity
# 출력 예시:
#   fileID=1234567890  name=Enemy  type=GameObject
#   fileID=9876543210  name=Enemy  type=GameObject  ← 동명 오브젝트

# 3. 이름이 아닌 fileID로 오브젝트를 특정해서 inspect
#    (이름만 알고 있으면 반드시 scene list 먼저 실행해서 fileID 확보)
unity-ctx scene inspect --scene Stage01.unity --file-id 1234567890
```

### 에셋 값 읽기 / 수정

```bash
# 값 읽기
unity-ctx asset get --file EnemyConfig.asset --field maxHealth

# dry-run 먼저
unity-ctx asset set --file EnemyConfig.asset --field maxHealth --value 200

# 확인 후 실제 반영
unity-ctx asset set --file EnemyConfig.asset --field maxHealth --value 200 --write
```

### 프리팹 수정

```bash
# GUID 확인
unity-ctx prefab guid --file Chair.prefab

# dry-run
unity-ctx prefab set --file Chair.prefab --prefab-guid <GUID> --field localPosition.y --value 0.5

# 반영 (--ack-impact 필수)
unity-ctx prefab set --file Chair.prefab --prefab-guid <GUID> --field localPosition.y --value 0.5 --write --ack-impact
```

### 씬 패치 적용

```bash
# 0. fileID를 모르면 먼저 scene list로 확보
unity-ctx scene list --scene Stage01.unity

# 1. patch 작성
unity-ctx scene patch --scene Stage01.unity --file-id 1234567890 --field ... --value ...

# 2. 반드시 diff로 내용 확인
unity-ctx scene diff --scene Stage01.unity

# 3. 확인 후 apply
unity-ctx scene apply --scene Stage01.unity
```

## 오류 대응

| 오류 코드 | 의미 | 대응 |
|-----------|------|------|
| `ERROR AMBIGUOUS_NAME` | 이름으로 오브젝트를 특정할 수 없음 | fileID로 재시도 |
| `WARN GUID_UNKNOWN` | prefab-guid 미지정 | `unity-ctx prefab guid`로 GUID 확보 후 재실행 |
| `ERROR EDITOR_NOT_RUNNING` | Editor 미실행 | Unity Editor를 실행하고 재시도 |
| `ERROR DIRTY_SCENE` | 미저장 변경사항 있음 | Editor에서 씬 저장 후 재시도 |

## 패턴에 없는 작업 요청 시

위 패턴에 해당하지 않는 작업(예: 씬에 프리팹 인스턴스 배치, 컴포넌트 추가 등)은 커맨드를 추측해서 실행하지 않는다. 반드시 아래 참조 문서를 먼저 읽고 올바른 커맨드를 확인한 뒤 실행한다.

## 참조 문서 라우팅

더 자세한 내용이 필요하면 아래 파일을 읽는다. (프로젝트 로컬 경로)

| 필요한 것 | 파일 경로 |
|-----------|-----------|
| 커맨드 전체 레퍼런스 | `.claude/skills/use-unity-ctx/references/commands.md` |
| 작업별 워크플로우 | `.claude/skills/use-unity-ctx/references/workflows.md` |
| 서브에이전트 템플릿 | `.claude/skills/use-unity-ctx/references/subagent-prompts.md` |

> **Note**: 위 참조 파일들은 프로젝트 로컬에 위치한다. 해당 파일이 없으면 사용자에게 알린다.
