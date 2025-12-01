# TODO - Synology File Cache

## 다음 작업 순서 (Next Steps)

### Step 2: 로컬 파일시스템 관리 (`fs` 패키지)
- [ ] `fs.Manager` 인터페이스 정의
- [ ] Synology 경로 ↔ 로컬 경로 매핑 구현
- [ ] 디렉토리 생성/관리 함수
- [ ] 파일 쓰기/읽기/삭제 함수
- [ ] 디스크 사용량 계산 로직
- [ ] 디스크 사용량 상한 체크 (GB & 퍼센트)

### Step 3: Synology HTTP API 클라이언트 (`synoapi`)
- [ ] Synology Drive API 문서 조사
- [ ] 인증/세션 관리 구현
  - [ ] 로그인 API
  - [ ] 토큰/세션 유지
  - [ ] 자동 재인증
- [ ] 파일 목록 조회 API
  - [ ] 공유된 파일/폴더 목록
  - [ ] 즐겨찾기(Starred) 목록
  - [ ] 최근 수정 파일 목록
  - [ ] 최근 접근 파일 목록
- [ ] 파일 다운로드 API
- [ ] 공유 링크 관리 API
  - [ ] 공유 목록 조회
  - [ ] 공유 상세 정보 조회
  - [ ] 공유 생성/해제

### Step 4: 동기화 로직 (`syncer`)
- [ ] `Syncer` 구조체 설계
- [ ] 전체 동기화 (`FullSync`)
  - [ ] Synology에서 파일 목록 가져오기
  - [ ] DB 업데이트 (upsert)
  - [ ] 우선순위 계산 로직
  - [ ] 삭제된 파일 처리
- [ ] 증분 동기화 (`IncrementalSync`)
  - [ ] 변경사항 감지
  - [ ] 부분 업데이트
- [ ] 공유 해제/삭제 규칙 구현
- [ ] 동기화 스케줄러
  - [ ] 주기적 실행
  - [ ] 에러 처리 및 재시도

### Step 5: 캐싱 엔진 (`cacher`)
- [ ] `Cacher` 구조체 설계
- [ ] 프리패치 계획 (`PlanPrefetch`)
  - [ ] 우선순위 기반 파일 선택
  - [ ] 디스크 공간 예측
- [ ] 프리패치 실행 (`RunPrefetchOnce`)
  - [ ] 파일 다운로드
  - [ ] 로컬 저장
  - [ ] DB 상태 업데이트
  - [ ] 진행상황 로깅
- [ ] Eviction 로직 (`Evict`)
  - [ ] LRU + 우선순위 기반 선택
  - [ ] 파일 삭제
  - [ ] DB 상태 업데이트
- [ ] 캐시 히트/미스 추적

### Step 6: HTTP API 완성
- [ ] 파일 다운로드 엔드포인트 구현
  - [ ] 토큰 검증
  - [ ] 만료/권한 체크
  - [ ] 캐시된 파일 서빙
  - [ ] 스트리밍 지원
  - [ ] Range 요청 지원
- [ ] 디버그 API 개선
  - [ ] 필터/정렬 파라미터
  - [ ] 페이지네이션
- [ ] 메트릭 엔드포인트
  - [ ] Prometheus 포맷 지원

### Step 7: 공유 링크 생성 이벤트 처리
- [ ] 프록시 엔드포인트 구현
  - [ ] `POST /proxy/share-create`
  - [ ] Synology API 호출 중계
  - [ ] DB 즉시 업데이트
  - [ ] 즉시 프리패치 트리거

## 추가 개선사항

### 성능 최적화
- [ ] 동시 다운로드 제한
- [ ] 대용량 파일 처리 최적화
- [ ] DB 쿼리 최적화
- [ ] 메모리 사용량 프로파일링

### 안정성
- [ ] 에러 처리 강화
- [ ] Panic recovery
- [ ] 재시도 로직
- [ ] Circuit breaker 패턴

### 운영
- [ ] 구조화된 로깅 개선
- [ ] 메트릭 수집 (Prometheus)
- [ ] Health check 상세화
- [ ] 설정 핫 리로드

### 테스트
- [ ] 유닛 테스트 작성
- [ ] 통합 테스트
- [ ] 부하 테스트
- [ ] Mock Synology API 서버

### 배포
- [ ] Dockerfile 작성
- [ ] Docker Compose 예제
- [ ] Kubernetes 매니페스트
- [ ] Helm chart
- [ ] CI/CD 파이프라인

### 문서화
- [ ] API 문서 (Swagger/OpenAPI)
- [ ] 설정 가이드 상세화
- [ ] 트러블슈팅 가이드
- [ ] 아키텍처 다이어그램

## 현재 상태

### ✅ 완료된 작업 (Step 0-1)
- [x] 프로젝트 구조 설정
- [x] Go 모듈 초기화
- [x] Config 패키지 (YAML 로딩/검증)
- [x] Logger 패키지 (zap 기반)
- [x] Store 패키지 (SQLite)
  - [x] 마이그레이션
  - [x] File/Share 모델
  - [x] CRUD 메서드
- [x] HTTP API 서버 스켈레톤
- [x] Main 애플리케이션
- [x] README.md 작성
- [x] GitHub 저장소 푸시

## 우선순위별 작업 분류

### 🔴 높음 (핵심 기능)
1. Synology API 클라이언트 (Step 3)
2. 동기화 로직 (Step 4)
3. 캐싱 엔진 (Step 5)
4. 파일시스템 관리 (Step 2)

### 🟡 중간 (완성도)
1. HTTP API 완성 (Step 6)
2. 공유 링크 처리 (Step 7)
3. 에러 처리 강화
4. 로깅 개선

### 🟢 낮음 (개선사항)
1. 테스트 작성
2. Docker 지원
3. 메트릭/모니터링
4. 문서화

## 예상 일정

- **Week 1**: Step 2-3 (fs 패키지, Synology API 기본)
- **Week 2**: Step 4 (동기화 로직)
- **Week 3**: Step 5 (캐싱 엔진)
- **Week 4**: Step 6-7 (API 완성, 공유 링크)
- **Week 5+**: 테스트, 최적화, 배포 준비

## 메모

- Synology Drive Server API 문서: https://global.download.synology.com/download/Document/Software/DeveloperGuide/Package/FileStation/All/enu/Synology_File_Station_API_Guide.pdf
- 실제 Synology API 엔드포인트 확인 필요 (브라우저 개발자 도구로 분석)
- 대용량 파일 처리 시 스트리밍 중요
- 동시 접속 제한 고려
- 캐시 warming up 전략 필요