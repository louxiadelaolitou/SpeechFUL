import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("backfill", Path(__file__).with_name("backfill.py"))
backfill = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backfill)


class BackfillBoundaryTests(unittest.TestCase):
    def test_rejects_other_origins_or_non_thumbnail_routes(self):
        post = "11111111-1111-4111-8111-111111111111"
        content = "22222222-2222-4222-8222-222222222222"
        route = f"/api/v1/posts/{post}/images/{content}/thumbnail"
        group = {"postId": post, "images": [{"contentVersionId": content, "thumbnail": {"path": route}}]}
        self.assertEqual(backfill.thumbnail_paths(post, group), [route])
        for path in ["https://other.example" + route, "//other.example" + route, route.replace("thumbnail", "original")]:
            group["images"][0]["thumbnail"]["path"] = path
            with self.assertRaises(ValueError):
                backfill.thumbnail_paths(post, group)

    def test_origin_and_duplicates_fail_closed(self):
        for url in ["http://example.com", "https://a:b@example.com", "https://example.com/path", "https://example.com?secret"]:
            with self.assertRaises(ValueError):
                backfill.origin(url)
        self.assertEqual(backfill.origin("https://example.com/"), "https://example.com")
        post = "11111111-1111-4111-8111-111111111111"
        content = "22222222-2222-4222-8222-222222222222"
        item = {"contentVersionId": content, "thumbnail": {"path": f"/api/v1/posts/{post}/images/{content}/thumbnail"}}
        with self.assertRaises(ValueError):
            backfill.thumbnail_paths(post, {"postId": post, "images": [item, item]})


if __name__ == "__main__":
    unittest.main()
