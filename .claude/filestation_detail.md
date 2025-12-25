# Synology File Station API 개발 가이드

이 문서는 Synology File Station API를 사용하여 실제 애플리케이션을 개발할 때 필요한 핵심 내용을 정리한 것입니다.

## 1. 기본 설정 및 워크플로우 (Chapter 2)

### 1.1 기본 요청 URL 구조

모든 API 요청은 HTTP/HTTPS GET 또는 POST 방식을 사용합니다.

```http
http://<Synology_IP>:<Port>/webapi/<CGI_PATH>?api=<API_NAME>&version=<VERSION>&method=<METHOD>[&<PARAMS>][&_sid=<SID>]

```

* **Port**: 기본적으로 HTTP는 5000, HTTPS는 5001입니다.
* **Encoding**: 모든 파라미터 값은 URL Encoding 되어야 합니다. 특히 JSON 형식의 배열(`[]`, `""`)이나 경로(`/`)가 포함될 때 주의해야 합니다 .


* 
**인증**: 로그인 후 발급받은 `sid`를 쿼리 파라미터 `_sid=<SID>`로 넘기거나, 쿠키(`id=<SID>`)에 포함해야 합니다 .



### 1.2 응답 형식 (JSON)

모든 응답은 JSON 형식으로 반환됩니다.

* **성공 시**:
```json
{
  "success": true,
  "data": { ... } // API별 상세 데이터
}

```


* **실패 시**:
```json
{
  "success": false,
  "error": {
    "code": 101,  // 에러 코드
    "errors": [ ... ] // (선택적) 파일별 상세 에러 정보
  }
}

```






### 1.3 주요 공통 에러 코드

개발 시 예외 처리가 필요한 주요 코드입니다. 

| 코드 | 설명 | 해결 방법 |
| --- | --- | --- |
| **101** | 파라미터 누락 | `api`, `version`, `method` 등 필수 파라미터 확인 |
| **105** | 권한 없음 | 로그인 세션이 유효하지 않음 (재로그인 필요) |
| **119** | SID 없음 | `_sid` 파라미터가 누락되었거나 만료됨 |
| **408** | 파일 없음 | 요청한 파일/폴더 경로가 존재하는지 확인 |
| **414** | 파일 중복 | 이미 존재하는 파일명으로 생성/업로드 시도 |
| **418** | 잘못된 경로 | 경로 형식이 맞지 않음 |

---

## 2. 인증 및 API 정보 (Chapter 3)

가장 먼저 구현해야 할 부분입니다.

### 2.1 API 정보 조회 (SYNO.API.Info)

각 API의 정확한 `CGI 경로(path)`와 `지원 버전(version)`을 알아냅니다. 

* **URL**: `/webapi/query.cgi`
* **파라미터**:
* `query`: `all` 또는 조회할 API 이름들 (예: `SYNO.FileStation.List,SYNO.API.Auth`)


* **응답 예시**:
```json
{
  "data": {
    "SYNO.FileStation.List": {
      "path": "FileStation/file_share.cgi",
      "minVersion": 1,
      "maxVersion": 2
    }
  },
  "success": true
}

```


> **Tip**: 개발 초기 단계에서 한 번 조회하여 상수로 관리하거나, 앱 시작 시 동적으로 조회하여 호환성을 확보합니다.



### 2.2 로그인 (SYNO.API.Auth)

세션을 생성하고 SID를 발급받습니다. 

* **URL**: `/webapi/auth.cgi`
* **Method**: `login`
* **파라미터**:
* `account`: 사용자 ID
* `passwd`: 사용자 비밀번호
* `session`: 세션 구분자 (예: `FileStation`)
* `format`: `sid` (응답 본문으로 SID 수신) 또는 `cookie` (헤더 쿠키로 수신). **개발 시엔 `sid` 추천.**


* **응답**:
```json
{
  "data": {
    "sid": "발급된_세션_ID"
  },
  "success": true
}

```



### 2.3 로그아웃

* **Method**: `logout`
* 
**파라미터**: `session` (로그인 시 사용한 이름) 



---

## 3. 파일 관리 (Chapter 4: File Station API)

### 3.1 파일/폴더 목록 조회 (SYNO.FileStation.List)

파일 탐색기의 핵심 기능입니다. 

* **URL**: `/webapi/FileStation/file_share.cgi` (API Info로 확인한 경로 사용)

#### A. 공유 폴더 목록 (`list_share`)

가장 상위의 공유 폴더들을 가져옵니다.

* **Method**: `list_share`
* **주요 파라미터**:
* `additional`: `real_path`, `owner`, `time`, `perm`, `volume_status` (추가 정보 요청)


* **예제**: `...&method=list_share&additional=real_path,owner,time`

#### B. 폴더 내 파일 목록 (`list`)

특정 폴더 안의 파일들을 조회합니다.

* **Method**: `list`
* **필수 파라미터**:
* `folder_path`: 조회할 폴더 경로 (예: `/video`)


* **선택 파라미터**:
* `offset`: 시작 인덱스 (기본 0)
* `limit`: 가져올 개수 (기본 0 = 전체)
* `sort_by`: `name`, `size`, `user`, `mtime`, `type` 등
* `sort_direction`: `asc`, `desc`
* `pattern`: 파일명 검색 패턴 (Glob 패턴)
* `filetype`: `file`, `dir`, `all` (기본 `all`)
* **`additional`**: `real_path` (실제 경로), `size` (크기), `owner` (소유자), `time` (수정 시간), `perm` (권한), `type` (확장자)
> **중요**: 파일 크기나 수정 시간을 UI에 표시하려면 반드시 `additional` 파라미터에 `size,time` 등을 포함해야 합니다.





#### C. 파일 상세 정보 (`getinfo`)

특정 파일 하나 또는 여러 개의 정보를 갱신할 때 사용합니다.

* **Method**: `getinfo`
* **파라미터**:
* `path`: 경로들의 배열 (JSON 문자열) 예: `["/video/file1.mp4", "/video/file2.txt"]`
* `additional`: 위와 동일



### 3.2 파일 다운로드 (SYNO.FileStation.Download)

* **Method**: `download`
* **파라미터**:
* `path`: 다운로드할 파일 경로. 여러 개일 경우 `path="/folder/file1,/folder/file2"` 형태(콤마 구분) 또는 JSON 배열 포맷. 여러 개면 zip으로 압축되어 다운로드됨.
* `mode`:
* `open`: 브라우저에서 바로 열기 시도 (이미지 등)
* `download`: 강제 다운로드 (attachment)




* 
**동작**: API 호출 시 파일 바이너리 스트림이 응답으로 옵니다. 



### 3.3 공유 링크 관리 (SYNO.FileStation.Sharing)

외부 사용자에게 파일을 공유할 때 사용합니다. 

* **URL**: API Info 참조
* **Method**: `create`
* **파라미터**:
* `path`: 공유할 파일/폴더 경로 (콤마로 구분하여 여러 개 가능)
* `password`: (선택) 접근 비밀번호
* `date_expired`: (선택) 만료일 (`YYYY-MM-DD` 형식, "0"은 무기한)
* `date_available`: (선택) 시작일


* **응답 데이터**:
```json
{
  "links": [
    {
      "id": "링크ID",
      "url": "http://gofile.me/...", // 공유 URL
      "qrcode": "Base64_QR_Code_String" // QR코드 이미지
    }
  ]
}

```



---

## 4. 추가 기능 (선택적 구현)

### 4.1 File Station 정보 (SYNO.FileStation.Info)

* **Method**: `get`
* 
**활용**: 현재 로그인한 사용자가 관리자인지(`is_manager`), 공유 링크 생성이 가능한지(`support_sharing`) 등을 확인하여 UI 메뉴를 제어할 때 유용합니다. 



### 4.2 즐겨찾기 (SYNO.FileStation.Favorite)

* **Method**: `list` (목록), `add` (추가), `delete` (삭제)
* **활용**: 파일 탐색기 좌측 '즐겨찾기' 메뉴 구현 시 사용합니다. `path`와 `name` 파라미터를 사용합니다. 



### 4.3 파일 검색 (SYNO.FileStation.Search)

비동기 방식으로 동작합니다. 

1. **검색 시작 (`start`)**: `folder_path`, `pattern`, `recursive`(재귀 여부) 등을 보내면 `taskid`를 응답받습니다.
2. **결과 조회 (`list`)**: `taskid`를 파라미터로 주기적으로 호출(Polling)합니다. 응답의 `finished: true`가 될 때까지 반복하며 `files` 목록을 받아옵니다.
3. **검색 종료 (`stop`/`clean`)**: 검색이 끝나면 리소스를 정리합니다.

## 5. 실제 개발 시 팁

1. **경로 처리**: 모든 경로는 `/`로 시작하는 절대 경로(공유 폴더 기준)여야 합니다. (예: `/music/pop/song.mp3`)
2. **JSON 파싱**: `additional` 정보를 요청하면 응답 객체 구조가 깊어지므로(`data.files[0].additional.owner.user`), Null Pointer Exception 방지를 위한 방어 코드가 필요합니다.
3. 
**날짜 포맷**: API에서 반환하는 시간(atime, mtime, ctime, crtime)은 **Unix Timestamp (초 단위)** 입니다.  앱에서 표시할 때 변환이 필요합니다.