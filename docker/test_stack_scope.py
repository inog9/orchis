import unittest

from stack import jxscout_scopes


def patterns(origins):
    scopes = jxscout_scopes(origins)
    assert scopes[::2] == ["--scope"] * (len(scopes) // 2)
    return scopes[1::2]


class JxScoutScopeTests(unittest.TestCase):
    def test_subdomain_includes_exact_apex_and_siblings(self):
        self.assertEqual(patterns(["https://sub.example.com"]), [
            "https://sub.example.com/*",
            "https://example.com/*",
            "https://*.example.com/*",
        ])

    def test_multilabel_public_suffixes(self):
        for suffix in ("co.id", "my.id"):
            with self.subTest(suffix=suffix):
                self.assertEqual(patterns([f"https://sub.example.{suffix}"]), [
                    f"https://sub.example.{suffix}/*",
                    f"https://example.{suffix}/*",
                    f"https://*.example.{suffix}/*",
                ])

    def test_private_suffix_does_not_expand_to_other_tenants(self):
        self.assertEqual(patterns(["https://juice-shop.herokuapp.com"]), [
            "https://juice-shop.herokuapp.com/*",
        ])

    def test_apex_ip_and_port(self):
        self.assertEqual(patterns(["https://example.com"]), ["https://example.com/*"])
        self.assertEqual(patterns(["http://127.0.0.1:8080"]), ["http://127.0.0.1:8080/*"])
        self.assertEqual(patterns(["https://sub.example.co.id:8443"]), [
            "https://sub.example.co.id:8443/*",
            "https://example.co.id:8443/*",
            "https://*.example.co.id:8443/*",
        ])


if __name__ == "__main__":
    unittest.main()
