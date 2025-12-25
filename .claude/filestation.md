# Synology File Station Official API Guide

**Copyright Synology Inc. All Rights Reserved.**

## Table of Contents

* **Chapter 1: Introduction**
* **Chapter 2: Getting Started**
    * API Workflow
    * Making Requests
    * Parsing Response
    * Common Error Codes
    * Working Example
* **Chapter 3: Base API**
    * SYNO.API.Info
    * SYNO.API.Auth
* **Chapter 4: File Station API**
    * SYNO.FileStation.Info
    * SYNO.FileStation.List
    * SYNO.FileStation.Search
    * SYNO.FileStation.VirtualFolder
    * SYNO.FileStation.Favorite
    * SYNO.FileStation.Thumb
    * SYNO.FileStation.DirSize
    * SYNO.FileStation.MD5
    * SYNO.FileStation.CheckPermission
    * SYNO.FileStation.Upload
    * SYNO.FileStation.Download
    * SYNO.FileStation.Sharing
    * SYNO.FileStation.CreateFolder
    * SYNO.FileStation.Rename
    * SYNO.FileStation.CopyMove
    * SYNO.FileStation.Delete
    * SYNO.FileStation.Extract
    * SYNO.FileStation.Compress
    * SYNO.FileStation.BackgroundTask
* **Appendix A: Release Notes**

---

## Chapter 1: Introduction

본 File Station 공식 API 개발자 가이드는 File Station의 API를 기반으로 애플리케이션을 확장하여 HTTP/HTTPS 요청 및 응답을 통해 DSM의 파일과 상호 작용하는 방법을 설명합니다. 이 문서는 다양한 File Station API의 구조와 상세 사양을 설명합니다.

"Chapter 2: Getting Started"에서는 API 사양을 살펴보기 전에 읽어보는 것이 좋은 API 사용 방법에 대한 기본 지침을 설명합니다. "Chapter 3: Base API" 및 "Chapter 4: File Station API"에는 사용 가능한 모든 API와 관련 세부 정보가 나열되어 있습니다.

---

## Chapter 2: Getting Started

File Station API를 사용하여 애플리케이션을 개발하기 전에 API 개념과 절차에 대한 기본적인 이해가 필요합니다. 이 장에서는 다음 5개 섹션으로 나누어 API 프로세스를 실행하고 완료하는 방법을 설명합니다.

* **API Workflow**: File Station API 작업 방법 소개
* **Making Requests**: API 요청 구성 방법 상세 설명
* **Parsing Response**: 응답 데이터 파싱 방법 설명
* **Common Error Codes**: 모든 File Station API에서 반환될 수 있는 공통 오류 코드 목록
* **Working Example**: 파일 작업 요청 예제 제공

### API Workflow

애플리케이션이 File Station API와 상호 작용하는 5단계 워크플로우는 다음과 같습니다.

1.  **Retrieve API Information (API 정보 검색)**: `SYNO.API.Info`를 통해 사용 가능한 API 목록과 경로 확인
2.  **Log in (로그인)**: `SYNO.API.Auth`를 통해 인증 및 세션 획득
3.  **Making API Requests (API 요청 생성)**: 필요한 기능(파일 목록, 업로드 등) 요청
4.  **Parse an API Response (응답 파싱)**: JSON 응답 처리
5.  **Log out (로그아웃)**: 세션 종료

#### Step 1: Retrieve API Information
애플리케이션은 먼저 대상 DiskStation에서 API 정보를 검색하여 어떤 API를 사용할 수 있는지 확인해야 합니다. 이 정보는 `SYNO.API.Info` API 파라미터를 사용하여 `/webapi/query.cgi`에 요청하여 액세스할 수 있습니다. 응답에는 사용 가능한 API 이름, 메서드, 경로 및 버전이 포함됩니다.

#### Step 2: Log in
File Station과 상호 작용하려면 계정과 비밀번호로 로그인해야 합니다. 로그인 프로세스는 `SYNO.API.Auth` API의 `login` 메서드를 호출하는 것입니다. 성공하면 승인된 세션 ID(`sid`)가 반환되며, 이후 API 요청 시 이 ID를 전달해야 합니다.

#### Step 3: Making API Requests
로그인에 성공하면 사용 가능한 모든 File Station API에 요청을 보낼 수 있습니다.

#### Step 4: Log out
작업을 마친 후 `SYNO.API.Auth` API의 `logout` 메서드를 호출하여 세션을 종료합니다.

### Making Requests

유효한 API 요청을 구성하는 5가지 기본 요소:
* **API name**: 요청된 API 이름 (예: `SYNO.API.Info`)
* **version**: API 버전
* **path**: API 경로 (`SYNO.API.Info` 요청으로 획득 가능)
* **method**: 요청된 API 메서드
* **_sid**: 승인된 세션 ID. `/webapi/auth.cgi` 응답에서 획득. HTTP/HTTPS GET/POST 메서드의 `_sid` 인자로 전달하거나 쿠키의 `id` 값으로 전달 가능.

**요청 구문:**
```http
GET /webapi/<CGI_PATH>?api=<API_NAME>&version=<VERSION>&method=<METHOD>[&<PARAMS>][&_sid=<SID>]