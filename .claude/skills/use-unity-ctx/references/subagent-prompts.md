# Sub-agent Prompt Templates

unity-ctx 작업을 서브에이전트에 위임할 때 사용하는 시작 프롬프트 템플릿.
변수는 `<대문자>`로 표시. 실제 사용 시 해당 값으로 교체한다.

---

## Template 1: Inspect & Report

씬/프리팹/에셋의 구조와 특정 오브젝트를 조사해서 보고할 때.

```
Unity 파일을 조사하고 결과를 보고해줘.

대상 파일: <FILE>  (예: Assets/Scenes/Stage01.unity)
조사할 오브젝트: <TARGET_NAME_OR_ID>  (이름 또는 fileID)
확인할 컴포넌트: <COMPONENT>  (예: Rigidbody, NavMeshAgent — 없으면 전체)

작업 순서:
1. `unity-ctx scene summarize <FILE>` 로 전체 구조 파악
2. `unity-ctx scene query <FILE> --name <TARGET_NAME_OR_ID>` 로 fileID 확보
   (이미 fileID를 알면 생략)
3. `unity-ctx scene inspect <FILE> --id <FILE_ID> --component <COMPONENT>` 로 상세 조회
4. 특정 필드 값이 필요하면 `unity-ctx scene get <FILE> --id <FILE_ID> --component <COMPONENT> --field <FIELD>`

규칙:
- .unity / .prefab / .asset 파일을 직접 읽지 않는다. 반드시 unity-ctx 커맨드를 사용한다.
- 오브젝트 특정은 fileID 우선. 이름이 중복될 수 있다.

조사 결과를 다음 형식으로 보고:
- fileID
- 컴포넌트 목록
- 요청된 필드 값
- WARN / UNKNOWN 출력이 있으면 함께 보고
```

---

## Template 2: Place Prefab

씬에 프리팹을 배치하는 전체 플로우 (scan → suggest → diff → apply).

```
Unity 씬에 프리팹을 배치해줘.

씬 파일: <SCENE>  (예: Assets/Scenes/Stage01.unity)
Unity 프로젝트 경로: <PROJECT_PATH>  (예: /Users/me/MyUnityProject)
Manifest 저장 경로: <MANIFEST>  (예: /tmp/Stage01.bounds.json)
배치할 프리팹: <PREFAB_PATH>  (예: Assets/Prefabs/Chair.prefab)
앵커 오브젝트: <ANCHOR>  (fileID 또는 이름, 예: Table_01)
프리팹 GUID: <PREFAB_GUID>  (예: abc-guid-123 — 모르면 UNKNOWN으로 표시)
Patch 저장 경로: <PATCH_FILE>  (예: /tmp/chair.patch.json)

GUID를 모르는 경우:
  grep "^guid:" <PROJECT_PATH>/Assets/Prefabs/<PREFAB>.prefab.meta

작업 순서:
1. scan — Unity Editor가 <PROJECT_PATH> 프로젝트를 열고 실행 중인지 확인 후:
   unity-ctx scene scan <SCENE> --mode editor --project <PROJECT_PATH> --out <MANIFEST>

2. suggest — 배치 후보 탐색 및 patch 파일 생성:
   unity-ctx scene suggest <SCENE> \
     --manifest <MANIFEST> \
     --prefab <PREFAB_PATH> \
     --near <ANCHOR> \
     --prefab-guid <PREFAB_GUID> \
     --out <PATCH_FILE>

3. diff — patch 내용 확인:
   unity-ctx scene diff <SCENE> --patch <PATCH_FILE>

4. apply — diff 확인 후 실제 적용:
   unity-ctx scene apply <SCENE> --patch <PATCH_FILE> --write

규칙:
- .unity 파일을 직접 편집하지 않는다.
- GUID가 UNKNOWN이면 apply를 실행하지 않는다.
- diff 없이 apply --write를 실행하지 않는다.
- scan은 Editor 실행 중 상태에서만 동작한다.

각 단계의 출력을 그대로 보고하고, ERROR나 WARN이 있으면 중단 후 보고한다.
```

---

## Template 3: Modify Field

에셋 또는 프리팹의 특정 필드 값을 수정할 때.

```
Unity 파일의 필드를 수정해줘.

대상 파일: <FILE>  (예: Assets/Configs/EnemyConfig.asset 또는 Assets/Prefabs/Enemy.prefab)
파일 타입: <TYPE>  (asset 또는 prefab)
Unity 프로젝트 경로: <PROJECT_PATH>  (prefab인 경우 필수, asset은 불필요)
FileID: <FILE_ID>  (예: 11400000 — 모르면 inspect로 확보)
필드 이름: <FIELD>  (예: maxHealth, moveSpeed)
새 값: <VALUE>  (예: 300, 4.0)

FileID를 모르는 경우:
  unity-ctx <TYPE> inspect <FILE> [--project <PROJECT_PATH>]

작업 순서 (asset인 경우):
1. 현재 값 확인:
   unity-ctx asset get <FILE> --field <FIELD>

2. dry-run:
   unity-ctx asset set <FILE> --id <FILE_ID> --field <FIELD> --value <VALUE>

3. DRY_RUN 출력의 old/new/type_hint 확인 후 실제 적용:
   unity-ctx asset set <FILE> --id <FILE_ID> --field <FIELD> --value <VALUE> --write

작업 순서 (prefab인 경우):
1. 영향 범위 파악:
   unity-ctx prefab impact <FILE> --project <PROJECT_PATH>

2. dry-run (impact 요약 자동 포함):
   unity-ctx prefab set <FILE> --project <PROJECT_PATH> --id <FILE_ID> --field <FIELD> --value <VALUE>

3. impact 범위와 DRY_RUN 출력 확인 후 실제 적용:
   unity-ctx prefab set <FILE> --project <PROJECT_PATH> --id <FILE_ID> --field <FIELD> --value <VALUE> --write --ack-impact

규칙:
- .asset / .prefab 파일을 직접 편집하지 않는다.
- prefab set --write 시 반드시 --ack-impact를 함께 전달한다.
- WARN IMPACT_DEPTH_LIMIT가 출력되면 중단 후 영향 범위 불명확함을 보고한다.
- dry-run 출력에서 changed=0이면 이미 같은 값이므로 write를 건너뛴다.

각 단계의 출력을 그대로 보고하고, ERROR나 WARN이 있으면 중단 후 보고한다.
```
