**Synology Office Suite API 가이드 (Synology Drive v2)**

제공해주신 `office-suite-api.synology.com`의 문서를 바탕으로 **Synology Drive API v2** 내용을 정리해 드립니다. 이 API는 기존의 `SYNO.FileStation` API와 달리 더 현대적인 RESTful 구조를 따르며, Synology Drive의 고유 기능(팀 폴더, 라벨, 오피스 파일 변환 등)에 최적화되어 있습니다.

> **참고**: 이 API는 **DSM 7.2.2 이상**, **Synology Drive 4.0.0 이상** 환경에서 사용 가능합니다.

---

# 1. 개요 (Overview)

Synology Drive API는 파일 관리, 공유, 검색, 팀 폴더 관리 등의 기능을 제공합니다.

* **Base URL**: `https://{nas_url}`
* **프로토콜**: HTTPS 권장
* **데이터 형식**: JSON

---

# 2. 인증 (Authorization)

API를 호출하기 위해서는 먼저 인증 세션 ID(`sid`)를 발급받아 **쿠키(Cookie)** 헤더에 포함해야 합니다.

### 2.1 로그인 (Sign in)

* **설명**: 사용자 계정으로 로그인하여 `sid`를 획득합니다. 기존 WebAPI의 `SYNO.API.Auth`를 사용하거나, Drive API v2의 전용 `POST Sign in` 엔드포인트를 사용할 수 있습니다.
* **요청 (Standard WebAPI 방식)**:
```http
GET /webapi/auth.cgi?api=SYNO.API.Auth&version=3&method=login&account={user}&passwd={password}&format=sid

```


* **응답**:
```json
{
  "success": true,
  "data": {
    "sid": "kABC12345..."
  }
}

```



### 2.2 인증 헤더 설정

획득한 `sid`를 이후 모든 API 요청의 헤더에 포함해야 합니다.

* **Header**: `Cookie: id={sid}`

---

# 3. 파일 작업 (Files)

파일 및 폴더에 대한 생성, 조회, 수정, 삭제 작업을 수행합니다.

### 3.1 파일/폴더 목록 조회 (Get files and folders)

* **Method**: `POST` (또는 `GET`)
* **기능**: 특정 폴더 내의 파일 목록을 가져옵니다.
* **주요 파라미터**:
* `path`: 조회할 폴더 경로 (예: `/My Drive/Projects`)
* `offset`, `limit`: 페이징 처리
* `sort_by`: 정렬 기준 (`name`, `time`, `size`, `type`)
* `sort_direction`: `asc` (오름차순) / `desc` (내림차순)
* `filter`: 파일 타입 등으로 필터링



### 3.2 파일/폴더 생성 (Create file or folder)

* **Method**: `POST`
* **기능**: 새 폴더를 만들거나 파일을 업로드(생성)합니다.
* **주요 파라미터**:
* `path`: 생성될 경로
* `name`: 파일/폴더 이름
* `type`: `folder` 또는 파일 타입
* `conflict_action`: 이름 충돌 시 동작 (`rename`, `overwrite`, `skip`)



### 3.3 파일 다운로드 (Download)

* **Method**: `POST` (또는 `GET`)
* **기능**: 파일을 다운로드합니다.
* **주요 파라미터**:
* `path`: 다운로드할 파일 경로. 여러 개의 경로를 전달하면 압축 파일(.zip)로 다운로드될 수 있습니다.



### 3.4 기타 작업

* **복사/이동 (Copy/Move)**: `POST Copy`, `POST Move` 엔드포인트를 통해 파일 위치를 변경합니다.
* **삭제 (Delete)**: `POST Delete`를 통해 휴지통으로 이동하거나 영구 삭제합니다.
* **검색 (Search)**: `POST Search`를 사용하여 키워드, 라벨, 날짜 등으로 파일을 검색합니다.

---

# 4. 공유 (Sharing)

외부 사용자 또는 내부 사용자와 파일을 공유하는 기능입니다.

### 4.1 공유 링크 생성 (Create sharing link)

* **Method**: `POST`
* **기능**: 파일이나 폴더에 대한 공개 공유 링크를 생성합니다.
* **주요 파라미터**:
* `path`: 공유할 대상 경로
* `password`: (선택) 보호 비밀번호
* `date_expired`: (선택) 만료 날짜 (`YYYY-MM-DD`)
* `date_available`: (선택) 시작 날짜



### 4.2 권한 관리 (Permissions)

* **Method**: `GET List permissions`, `PUT Update permissions`
* **기능**: 특정 파일에 대해 어떤 사용자가 접근 권한을 가지고 있는지 확인하고 수정합니다.

---

# 5. 팀 폴더 (Team Folders)

Synology Drive의 핵심 협업 기능인 팀 폴더를 관리합니다.

* **팀 폴더 목록 (List Team Folders)**: 활성화된 팀 폴더 목록을 조회합니다.
* **멤버 관리 (Get Members)**: 특정 팀 폴더에 접근 가능한 멤버 목록을 확인합니다.

---

# 6. 기타 기능 (Labels & Misc)

* **라벨 (Labels)**: 파일에 색상 라벨을 지정하거나 라벨별로 파일을 모아보는 기능을 제공합니다.
* **별표 (Stars)**: `POST Stars` 등을 통해 중요 파일을 즐겨찾기(별표) 합니다.
* **웹훅 (Webhooks)**: 파일 변경 사항(생성, 삭제 등) 발생 시 외부 시스템으로 알림을 보낼 수 있도록 웹훅을 설정합니다.

---

# 개발 시 참고 사항

1. **엔드포인트 경로**: Synology Drive v2 API는 기존 WebAPI (`entry.cgi`) 방식과 달리 `/api/v2/...` 형태의 직관적인 REST 경로를 가질 수 있습니다. 정확한 경로는 제공해주신 [API 문서 페이지](https://office-suite-api.synology.com/Synology-Drive/v2)의 "Operations" 탭에서 확인하거나 `SYNO.API.Info`를 통해 조회해야 합니다.
2. **에러 처리**: HTTP 상태 코드(200, 4xx, 5xx)와 함께 JSON 바디 내의 `code` 값을 확인하여 상세 에러 원인(권한 없음, 파일 중복 등)을 파악해야 합니다.
3. **테스트**: Synology는 Postman 컬렉션을 제공하거나 문서 내에서 'Interactive Console'을 제공하는 경우가 많으므로, 실제 코드를 작성하기 전에 이를 활용하는 것이 좋습니다.

Here is a video about using Synology Drive APIs for automation [How to Enhance File Management with Synology Drive API Integration](https://www.youtube.com/watch?v=PG--Ge7tYfg). This video demonstrates how to access the documentation and integrate Synology Drive with tools like Trello using the API.