// Credentials enter only through this isolated package's private cache.
import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';
import 'package:hnuhole_mobile/src/design_system/telegram_theme.dart';

class _MeasuredVault implements AuthVault {
  final _delegate = FlutterAuthVault(namespace: 'req004.device.acceptance');
  int reads = 0, writes = 0, readMicros = 0, writeMicros = 0;
  @override
  Future<String?> read() async {
    final watch = Stopwatch()..start();
    try {
      return await _delegate.read();
    } finally {
      reads++;
      readMicros += watch.elapsedMicroseconds;
    }
  }

  @override
  Future<void> write(String value) async {
    final watch = Stopwatch()..start();
    try {
      await _delegate.write(value);
    } finally {
      writes++;
      writeMicros += watch.elapsedMicroseconds;
    }
  }
}

void main() {
  final binding = IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  testWidgets('real homepage visible thumbnails paired device performance', (
    tester,
  ) async {
    final root = Directory.systemTemp.path;
    final data = jsonDecode(
      await File('$root/req004-fixture.json').readAsString(),
    ) as Map<String, dynamic>;
    HttpClient secure() => HttpClient(
      context: SecurityContext(withTrustedRoots: false)
        ..setTrustedCertificatesBytes(
          utf8.encode(data['certificatePem'] as String),
        ),
    );
    final c = Uri.parse(data['community'] as String),
        v = Uri.parse(data['verifier'] as String);
    final auth = HttpAuthApi(
      communityBaseUri: c,
      verifierBaseUri: v,
      client: secure(),
    );
    final vault = _MeasuredVault();
    final store = AuthStore(vault);
    final token = data['token'] as String;
    final current = await auth.currentSession(token);
    await store.update(
      (s) => s.copyWith(
        session: SessionRecord(
          token: token,
          accountId: current.accountId,
          expiresAt: current.expiresAt,
        ),
      ),
    );
    final directory = ChannelDirectoryController(
      repository: HttpChannelRepository(baseUri: c, client: secure()),
    );
    final sessions = AuthSessionController(
      api: auth,
      store: store,
      directory: directory,
    );
    await sessions.start();
    final samples = <Map<String, Object>>[];
    for (var round = 0; round < 8; round++) {
      for (final reuse in round.isEven ? [false, true] : [true, false]) {
        final control = HttpClient();
        final request = await control.postUrl(
          Uri.parse('${data['control']}/${reuse ? 'reuse' : 'baseline'}'),
        );
        final response = await request.close();
        expect(response.statusCode, 204);
        await response.drain<void>();
        control.close();
        PaintingBinding.instance.imageCache.clear();
        PaintingBinding.instance.imageCache.clearLiveImages();
        final api = HttpPostApi(communityBaseUri: c, client: secure());
        final posts = PostController(
          api: api,
          sessions: sessions,
          authStore: store,
          identities: auth,
          openStore: (_) async => throw const PostStorageFailure(),
          pollInterval: Duration.zero,
          mediaApi: api,
        );
        final media = PostMediaRepository(posts, maxIdleEntries: 0);
        final readsBefore = vault.reads, writesBefore = vault.writes;
        final readMicrosBefore = vault.readMicros,
            writeMicrosBefore = vault.writeMicros;
        final watch = Stopwatch()..start();
        int? feedReadyMicros, manifestReadyMicros;
        posts.addListener(() {
          if (posts.timelineItems.isNotEmpty) {
            feedReadyMicros ??= watch.elapsedMicroseconds;
          }
        });
        await tester.pumpWidget(
          MaterialApp(
            theme: HhTelegramTheme.create(Brightness.light),
            home: FeedScreen(
              controller: posts,
              channels: directory.channels,
              onManageIdentities: () async {},
              onAuthenticationRequired: () => fail('session lost'),
              loadImages: (postId) async {
                final group = await media.load(postId);
                manifestReadyMicros ??= watch.elapsedMicroseconds;
                return group;
              },
            ),
          ),
        );
        var rendered = false;
        while (watch.elapsed < const Duration(seconds: 20)) {
          await tester.pump(const Duration(milliseconds: 16));
          final images = find.descendant(
            of: find.byType(PostImagesSection),
            matching: find.byType(RawImage),
          );
          final visible = images.evaluate().where((e) {
            final r = e.renderObject;
            return r is RenderImage &&
                r.attached &&
                r.paintBounds
                    .shift(r.localToGlobal(Offset.zero))
                    .overlaps(
                      Offset.zero &
                          (binding.platformDispatcher.views.first.physicalSize /
                              binding
                                  .platformDispatcher
                                  .views
                                  .first
                                  .devicePixelRatio),
                    );
          }).toList();
          if (visible.isNotEmpty &&
              visible.every(
                (e) => (e.renderObject! as RenderImage).image != null,
              )) {
            await tester.pump();
            rendered = true;
            break;
          }
        }
        watch.stop();
        expect(rendered, true, reason: 'visible product images did not paint');
        if (round >= 2) {
          samples.add({
            'round': round - 1,
            'reuse': reuse,
            'microseconds': watch.elapsedMicroseconds,
            'feedReadyMicros': feedReadyMicros ?? -1,
            'manifestReadyMicros': manifestReadyMicros ?? -1,
            'vaultReads': vault.reads - readsBefore,
            'vaultWrites': vault.writes - writesBefore,
            'vaultReadMicros': vault.readMicros - readMicrosBefore,
            'vaultWriteMicros': vault.writeMicros - writeMicrosBefore,
          });
        }
        await tester.pumpWidget(const SizedBox.shrink());
        await tester.pump();
        media.dispose();
        posts.dispose();
        api.close();
      }
    }
    final baseline =
        samples
            .where((s) => s['reuse'] == false)
            .map((s) => s['microseconds'] as int)
            .toList()
          ..sort();
    final reuse =
        samples
            .where((s) => s['reuse'] == true)
            .map((s) => s['microseconds'] as int)
            .toList()
          ..sort();
    final reduction =
        1 - reuse[reuse.length ~/ 2] / baseline[baseline.length ~/ 2];
    await File('$root/req004-result.json').writeAsString(
      jsonEncode({
        'samples': samples,
        'medianReduction': reduction,
        'status': reduction >= .3 ? 'PASS' : 'FAIL',
        'scope': 'real FeedScreen; native vault; current bearer; actual TLS/C Gate; visible decoded images painted',
      }),
      flush: true,
    );
    sessions.dispose();
    directory.dispose();
    store.dispose();
    expect(reduction, greaterThanOrEqualTo(.3));
  });
}
