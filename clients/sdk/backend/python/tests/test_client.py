import json
import pytest
import responses

from progressdb.client import ProgressDBClient, ApiError


BASE = "http://api.test"


def test_api_error_fields():
    err = ApiError(400, {"error": "bad"})
    assert err.status == 400
    assert err.body == {"error": "bad"}


@responses.activate
def test_request_parses_json():
    responses.add(responses.GET, f"{BASE}/json", json={"ok": True}, status=200)
    c = ProgressDBClient(base_url=BASE)
    res = c.request("GET", "/json")
    assert res == {"ok": True}


@responses.activate
def test_request_returns_text_for_non_json():
    responses.add(responses.GET, f"{BASE}/text", body="hello", status=200, content_type="text/plain")
    c = ProgressDBClient(base_url=BASE)
    res = c.request("GET", "/text")
    assert res == "hello"


@responses.activate
def test_request_raises_api_error_on_4xx():
    responses.add(responses.GET, f"{BASE}/bad", json={"err": "x"}, status=400)
    c = ProgressDBClient(base_url=BASE)
    with pytest.raises(ApiError) as ei:
        c.request("GET", "/bad")
    assert ei.value.status == 400


@responses.activate
def test_create_thread_sends_x_user_id_header():
    def request_callback(request):
        headers = request.headers
        body = json.loads(request.body)
        return (200, {}, json.dumps({"id": "t1", "title": body.get("title"), "author": headers.get("X-User-ID")}))

    responses.add_callback(responses.POST, f"{BASE}/v1/threads", callback=request_callback, content_type="application/json")
    c = ProgressDBClient(base_url=BASE, api_key="k")
    t = c.create_thread({"title": "hello"}, author="author1")
    assert t["id"] == "t1"
    assert t["author"] == "author1"

