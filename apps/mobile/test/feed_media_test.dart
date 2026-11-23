import 'dart:async';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_image_viewer/image_viewer.dart';
import 'package:hnuhole_mobile/src/posts/post_images.dart';

import 'feed_ui_test.dart' as fixture;

import 'package:hnuhole_mobile/src/posts/post_controller.dart';

void main() {
  late MemoryImage image;
  setUpAll(() async {
    image = MemoryImage(
      await File(
        '../../packages/image_viewer/example/assets/fixtures/ordinary.png',
      ).readAsBytes(),
    );
  });
  PostImage media(
    String id,
    Size size, {
    Future<ImageProvider>? pending,
    bool animated = false,
  }) => PostImage.deferred(
    pixelSize: size,
    animated: animated,
    loadThumbnail: () => pending ?? Future.value(image),
    item: ViewerItem.provider(
      id: id,
      provider: image,
      pixelSize: size,
      saveAllowed: false,
    ),
  );
  PostImages group(
    List<PostImage> images,
    ValueNotifier<bool> access, {
    VoidCallback? release,
  }) => PostImages(
    postId: fixture.postId,
    images: images,
    accessGranted: access,
    onRelease: release,
  );
  Widget section(
    ValueNotifier<bool> authority,
    PostImages value, {
    double inset = 0,
  }) => MaterialApp(
    home: Scaffold(
      body: Align(
        alignment: Alignment.topLeft,
        child: SizedBox(
          width: 317,
          child: PostImagesSection(
            postId: fixture.postId,
            expectedCount: value.images.length,
            feedPreview: true,
            feedPreviewInset: inset,
            loadImages: (_) async => value,
            authority: authority,
            isCurrent: () => authority.value,
          ),
        ),
      ),
    ),
  );
  Future<void> frames(WidgetTester tester) async {
    for (var i = 0; i < 6; i++) {
      await tester.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 10)),
      );
      await tester.pump(const Duration(milliseconds: 40));
    }
  }

  for (final decodeFailure in [false, true]) {
    testWidgets(
      'thumbnail transfer/decode failure can explicitly retry: decode=$decodeFailure',
      (tester) async {
        final authority = ValueNotifier(true), access = ValueNotifier(true);
        var calls = 0;
        Future<ImageProvider> load() async {
          if (++calls > 1) return image;
          if (!decodeFailure) throw StateError('temporary transfer failure');
          return MemoryImage(Uint8List.fromList([1, 2, 3]));
        }

        final value = group([
          PostImage.deferred(
            pixelSize: const Size(640, 480),
            loadThumbnail: load,
            item: ViewerItem.provider(
              id: 'retry',
              provider: image,
              pixelSize: const Size(640, 480),
              saveAllowed: false,
            ),
          ),
        ], access);
        await tester.pumpWidget(section(authority, value));
        await frames(tester);
        expect(find.text('图片加载失败'), findsOneWidget);
        await tester.tap(find.text('点击重试'));
        await frames(tester);
        expect(calls, 2);
        expect(find.text('图片加载失败'), findsNothing);
        expect(find.byType(Image), findsOneWidget);
        expect(find.byType(CircularProgressIndicator), findsNothing);
        expect(tester.takeException(), isNull);
        await tester.pumpWidget(const SizedBox());
        authority.dispose();
        access.dispose();
      },
    );
  }

  testWidgets(
    'feed geometry is reserved before manifest and bytes; decoding stays bounded',
    (tester) async {
      final authority = ValueNotifier(true), access = ValueNotifier(true);
      final pending = Completer<PostImages>();
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: SingleChildScrollView(
              child: SizedBox(
                width: 317,
                child: PostImagesSection(
                  postId: fixture.postId,
                  expectedCount: 1,
                  feedPreview: true,
                  initialSizes: const [Size(1600, 900)],
                  loadImages: (_) => pending.future,
                  authority: authority,
                  isCurrent: () => authority.value,
                ),
              ),
            ),
          ),
        ),
      );
      final before = tester.getSize(find.byType(PostImagesSection));
      expect(before.height, closeTo(317 * 900 / 1600 + 2, .1));
      pending.complete(group([media('sized', const Size(1600, 900))], access));
      await frames(tester);
      expect(tester.getSize(find.byType(PostImagesSection)), before);
      final decoded =
          tester.widget<Image>(find.byType(Image).first).image as ResizeImage;
      expect(decoded.policy, ResizeImagePolicy.fit);
      expect(decoded.width, lessThanOrEqualTo(1280));
      expect(decoded.height, lessThanOrEqualTo(1280));
      await tester.pumpWidget(const SizedBox());
      authority.dispose();
      access.dispose();
    },
  );

  testWidgets('animated originals have a visible label on static previews', (
    tester,
  ) async {
    final authority = ValueNotifier(true), access = ValueNotifier(true);
    await tester.pumpWidget(
      section(
        authority,
        group([
          media('animated', const Size(640, 480), animated: true),
        ], access),
      ),
    );
    await frames(tester);
    expect(find.text('动图'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    authority.dispose();
    access.dispose();
  });

  testWidgets(
    'profile inset keeps the first image aligned to text and bleeds right',
    (tester) async {
      final authority = ValueNotifier(true), access = ValueNotifier(true);
      // 单图：两侧各留 20，与标题正文对齐（与首页一致，只是内缩）。
      await tester.pumpWidget(
        section(
          authority,
          group([media('single', const Size(900, 1200))], access),
          inset: 20,
        ),
      );
      await frames(tester);
      final single = find.byKey(const ValueKey(('feed-image', 'single')));
      expect(tester.getTopLeft(single).dx, 20);
      expect(tester.getSize(single).width, 277);
      await tester.pumpWidget(const SizedBox());

      // 多图：未滑动时第一张左缘与文字对齐，滑动后可越过 20 到屏幕左缘。
      await tester.pumpWidget(
        section(
          authority,
          group([
            media('one', const Size(900, 1200)),
            media('two', const Size(1600, 900)),
          ], access),
          inset: 20,
        ),
      );
      await frames(tester);
      final first = find.byKey(const ValueKey(('feed-image', 'one')));
      expect(tester.getTopLeft(first).dx, 20);
      final rail = find.byKey(const ValueKey('feed-media:${fixture.postId}'));
      await tester.drag(rail, const Offset(-200, 0));
      await tester.pumpAndSettle();
      expect(tester.getTopLeft(first).dx, lessThan(0));
      await tester.pumpWidget(const SizedBox());
      authority.dispose();
      access.dispose();
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'single image keeps its original ratio; only bounded extremes crop with a hint',
    (tester) async {
      final authority = ValueNotifier(true), access = ValueNotifier(true);
      for (final size in [
        const Size(1600, 900),
        const Size(900, 1200),
        const Size(900, 1600),
        const Size(900, 900),
      ]) {
        await tester.pumpWidget(
          section(authority, group([media('single', size)], access)),
        );
        await frames(tester);
        final actual = tester.getSize(
          find.byKey(const ValueKey(('feed-image', 'single'))),
        );
        final ratio = size.width / size.height;
        expect(actual.width, 317);
        // 正常比例：宽度铺满、高度严格按原比例，不裁切也不提示。
        expect(actual.height, closeTo(317 / ratio, .1));
        expect(find.text('点击查看完整图片'), findsNothing);
        expect(tester.widget<Image>(find.byType(Image)).fit, BoxFit.cover);
        await tester.pumpWidget(const SizedBox());
      }
      for (final (size, height) in [
        // 超出 1:2 的极端竖图：限幅后高度封顶 480，裁切并提示查看完整图片。
        (const Size(100, 3000), 480.0),
        // 超宽图：宽高比限幅到 3:1，高度由限幅比例决定，同样裁切并提示。
        (const Size(3000, 100), 317 / 3),
      ]) {
        await tester.pumpWidget(
          section(authority, group([media('single', size)], access)),
        );
        await frames(tester);
        final card = find.byKey(const ValueKey(('feed-image', 'single')));
        final actual = tester.getSize(card);
        expect(actual.width, 317);
        expect(actual.height, closeTo(height, .1));
        expect(find.text('点击查看完整图片'), findsOneWidget);
        // 裁切由 cover 在固定卡片内完成，不会变形或留边。
        expect(
          tester
              .widget<Image>(
                find.descendant(of: card, matching: find.byType(Image)),
              )
              .fit,
          BoxFit.cover,
        );
        await tester.pumpWidget(const SizedBox());
      }
      authority.dispose();
      access.dispose();
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'multi image row keeps one height and each original ratio without bars',
    (tester) async {
      final authority = ValueNotifier(true), access = ValueNotifier(true);
      final height = (317 * .8).clamp(160.0, 260.0);
      Future<void> expectRow(List<Size> sizes, List<double> ratios) async {
        await tester.pumpWidget(
          section(
            authority,
            group([
              for (var i = 0; i < sizes.length; i++)
                media('multi-$i', sizes[i]),
            ], access),
          ),
        );
        await frames(tester);
        for (var i = 0; i < sizes.length; i++) {
          final card = find.byKey(ValueKey(('feed-image', 'multi-$i')));
          final actual = tester.getSize(card);
          expect(actual.height, closeTo(height, .1));
          expect(actual.width, closeTo(height * ratios[i], .1));
          // 卡片宽高比与期望的（限幅后）原图一致，cover 时没有信箱留白或拉伸。
          expect(actual.width / actual.height, closeTo(ratios[i], .001));
          expect(
            tester
                .widget<Image>(
                  find.descendant(of: card, matching: find.byType(Image)),
                )
                .fit,
            BoxFit.cover,
          );
        }
        await tester.pumpWidget(const SizedBox());
      }

      // 正常比例：逐张按原宽高比计算宽度，等高且无留白。
      await expectRow(
        const [Size(900, 1200), Size(1600, 900)],
        const [900 / 1200, 1600 / 900],
      );
      // 极端比例：限幅到 1:2 与 3:1，只做有界裁切，仍保持同一行等高。
      await expectRow(const [Size(100, 3000), Size(3000, 100)], const [.5, 3]);
      authority.dispose();
      access.dispose();
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'FA-O02 expired single and carousel keep geometry without protected images',
    (tester) async {
      for (final sizes in [
        [const Size(1600, 900)],
        [const Size(900, 1200)],
        [const Size(100, 3000)],
        [const Size(900, 1200), const Size(1600, 900), const Size(900, 900)],
      ]) {
        final authority = ValueNotifier(true), access = ValueNotifier(true);
        await tester.pumpWidget(
          section(
            authority,
            group([
              for (var i = 0; i < sizes.length; i++)
                media('image-$i', sizes[i]),
            ], access),
          ),
        );
        await frames(tester);
        final before = tester.getSize(find.byType(PostImagesSection));
        access.value = false;
        await tester.pumpAndSettle();
        expect(tester.getSize(find.byType(PostImagesSection)), before);
        expect(find.byType(Image, skipOffstage: false), findsNothing);
        expect(find.byType(RawImage, skipOffstage: false), findsNothing);
        expect(find.text('图片已不可访问'), findsWidgets);
        await tester.pumpWidget(const SizedBox());
        authority.dispose();
        access.dispose();
      }
    },
  );
  testWidgets(
    'FA-O02 reload pending and failure retain known geometry and late bytes stay hidden',
    (tester) async {
      final authority = ValueNotifier(true),
          oldAccess = ValueNotifier(true),
          newAccess = ValueNotifier(true);
      final pendingBytes = Completer<ImageProvider>();
      final oldGroup = group([
        media('old', const Size(1600, 900), pending: pendingBytes.future),
      ], oldAccess);
      final freshGroup = group([
        media('fresh', const Size(1600, 900)),
      ], newAccess);
      var calls = 0;
      final reload = Completer<PostImages>();
      Future<PostImages> loader(String _) async {
        calls++;
        if (calls == 1) return oldGroup;
        if (calls == 2) return reload.future;
        return freshGroup;
      }

      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: SizedBox(
              width: 317,
              child: PostImagesSection(
                postId: fixture.postId,
                expectedCount: 1,
                feedPreview: true,
                loadImages: loader,
                authority: authority,
                isCurrent: () => authority.value,
              ),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      final before = tester.getSize(find.byType(PostImagesSection));
      oldAccess.value = false;
      await tester.pumpAndSettle();
      pendingBytes.complete(image);
      await tester.pumpAndSettle();
      expect(find.byType(Image), findsNothing);
      await tester.tap(find.text('重新读取图片'));
      await tester.pump();
      expect(tester.getSize(find.byType(PostImagesSection)), before);
      expect(find.byType(Image), findsNothing);
      reload.completeError(StateError('synthetic unavailable'));
      await tester.pumpAndSettle();
      expect(tester.getSize(find.byType(PostImagesSection)), before);
      expect(find.byType(Image), findsNothing);
      await tester.tap(find.text('重新加载图片'));
      await frames(tester);
      expect(tester.getSize(find.byType(PostImagesSection)), before);
      expect(find.byType(Image), findsOneWidget);
      expect(calls, 3);
      authority.value = false;
      await tester.pumpAndSettle();
      expect(tester.getSize(find.byType(PostImagesSection)).height, 0);
      expect(find.byType(Image), findsNothing);
      await tester.pumpWidget(const SizedBox());
      authority.dispose();
      oldAccess.dispose();
      newAccess.dispose();
    },
  );
  for (final expiresDuringGesture in [true, false]) {
    testWidgets(
      'FA-O02 lease changes during pointer gesture never open parent or viewer ($expiresDuringGesture)',
      (tester) async {
        final authority = ValueNotifier(true),
            oldAccess = ValueNotifier(true),
            freshAccess = ValueNotifier(true);
        final oldGroup = group([
          media('old', const Size(1600, 900)),
        ], oldAccess);
        final freshGroup = group([
          media('fresh', const Size(1600, 900)),
        ], freshAccess);
        final reload = Completer<PostImages>();
        var calls = 0, parentTaps = 0;
        Future<PostImages> loader(String _) async =>
            ++calls == 1 ? oldGroup : reload.future;
        await tester.pumpWidget(
          MaterialApp(
            home: Scaffold(
              body: InkWell(
                onTap: () => parentTaps++,
                child: SizedBox(
                  width: 317,
                  child: PostImagesSection(
                    postId: fixture.postId,
                    expectedCount: 1,
                    feedPreview: true,
                    loadImages: loader,
                    authority: authority,
                    isCurrent: () => authority.value,
                  ),
                ),
              ),
            ),
          ),
        );
        await frames(tester);
        final position = tester.getCenter(
          find.byKey(const ValueKey(('feed-image', 'old'))),
        );
        if (!expiresDuringGesture) {
          oldAccess.value = false;
          await tester.pump();
          await tester.tap(find.text('重新读取图片'));
          await tester.pump();
        }
        final gesture = await tester.startGesture(position);
        await tester.pump();
        if (expiresDuringGesture) {
          oldAccess.value = false;
          await tester.pump();
          expect(find.byType(Image, skipOffstage: false), findsNothing);
        } else {
          reload.complete(freshGroup);
          await frames(tester);
        }
        await gesture.up();
        await tester.pumpAndSettle();
        expect(parentTaps, 0);
        expect(find.byType(HnuImageViewer), findsNothing);
        expect(tester.takeException(), isNull);
        await tester.pumpWidget(const SizedBox());
        authority.dispose();
        oldAccess.dispose();
        freshAccess.dispose();
      },
    );
  }
  testWidgets(
    'FA-O02 narrow large-text status scrolls to retry without overflow',
    (tester) async {
      for (final scale in [2.0, 3.0]) {
        final authority = ValueNotifier(true), access = ValueNotifier(true);
        final value = group([media('wide', const Size(1600, 800))], access);
        final pending = Completer<PostImages>();
        var calls = 0;
        Future<PostImages> loader(String _) async =>
            ++calls == 1 ? value : pending.future;
        await tester.pumpWidget(
          MaterialApp(
            builder: (context, child) => MediaQuery(
              data: MediaQuery.of(context)
                  .copyWith(textScaler: TextScaler.linear(scale)),
              child: child!,
            ),
            home: Scaffold(
              body: Align(
                alignment: Alignment.topLeft,
                child: SizedBox(
                  width: 180,
                  child: PostImagesSection(
                    postId: fixture.postId,
                    expectedCount: 1,
                    feedPreview: true,
                    loadImages: loader,
                    authority: authority,
                    isCurrent: () => authority.value,
                  ),
                ),
              ),
            ),
          ),
        );
        await frames(tester);
        final before = tester.getSize(find.byType(PostImagesSection));
        access.value = false;
        await tester.pumpAndSettle();
        expect(tester.getSize(find.byType(PostImagesSection)), before);
        expect(tester.takeException(), isNull);
        await tester.ensureVisible(find.text('重新读取图片'));
        await tester.tap(find.text('重新读取图片'));
        await tester.pump();
        expect(calls, 2);
        expect(tester.getSize(find.byType(PostImagesSection)), before);
        expect(tester.takeException(), isNull);
        pending.completeError(StateError('synthetic unavailable'));
        await tester.pumpAndSettle();
        expect(tester.getSize(find.byType(PostImagesSection)), before);
        expect(tester.takeException(), isNull);
        await tester.ensureVisible(find.text('重新加载图片'));
        expect(find.text('重新加载图片').hitTestable(), findsOneWidget);
        expect(find.byType(Image, skipOffstage: false), findsNothing);
        await tester.pumpWidget(const SizedBox());
        authority.dispose();
        access.dispose();
      }
    },
  );
  testWidgets('metadata fixes geometry while image bytes load or fail', (
    tester,
  ) async {
    final authority = ValueNotifier(true), access = ValueNotifier(true);
    final pending = Completer<ImageProvider>();
    await tester.pumpWidget(
      section(
        authority,
        group([
          media('pending', const Size(1600, 900), pending: pending.future),
        ], access),
      ),
    );
    await tester.pumpAndSettle();
    final before = tester.getSize(
      find.byKey(const ValueKey(('feed-image', 'pending'))),
    );
    pending.completeError(StateError('fixture decode unavailable'));
    await tester.pumpAndSettle();
    expect(
      tester.getSize(find.byKey(const ValueKey(('feed-image', 'pending')))),
      before,
    );
    expect(find.text('图片加载失败'), findsOneWidget);
    expect(find.text('点击重试'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    authority.dispose();
    access.dispose();
  });
  testWidgets(
    'horizontal mixed media drags independently, opens correct index and revokes',
    (tester) async {
      final authority = ValueNotifier(true), access = ValueNotifier(true);
      var released = 0;
      await tester.pumpWidget(
        section(
          authority,
          group(
            [
              media('one', const Size(900, 1200)),
              media('two', const Size(1600, 900)),
              media('three', const Size(900, 900)),
            ],
            access,
            release: () => released++,
          ),
        ),
      );
      await frames(tester);
      final rail = find.byKey(const ValueKey('feed-media:${fixture.postId}'));
      final second = find.byKey(const ValueKey(('feed-image', 'two')));
      expect(tester.getTopLeft(second).dx, lessThan(317));
      final before = tester.getTopLeft(second).dx;
      await tester.drag(rail, const Offset(-170, 0));
      await tester.pumpAndSettle();
      expect(tester.getTopLeft(second).dx, lessThan(before));
      // 等高于容器、宽度随原比例，第二张可能比可视宽度更宽：点可视区内。
      await tester.tapAt(tester.getTopLeft(second) + const Offset(20, 40));
      await frames(tester);
      final viewer = tester.widget<HnuImageViewer>(find.byType(HnuImageViewer));
      expect(viewer.controller.index, 1);
      access.value = false;
      await tester.pumpAndSettle();
      expect(find.byType(HnuImageViewer), findsNothing);
      await tester.pumpWidget(const SizedBox());
      expect(released, 1);
      authority.dispose();
      access.dispose();
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'render production feed and controls with synthetic protected media',
    (tester) async {
      tester.view.physicalSize = const Size(402, 806);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      for (final entry in {
        'Roboto': 'HNUHOLE_UI_RENDER_LATIN_FONT',
        'HhNotoSansSC': 'HNUHOLE_UI_RENDER_FONT',
        'MaterialIcons': 'HNUHOLE_UI_RENDER_ICON_FONT',
      }.entries) {
        final path = Platform.environment[entry.value];
        if (path != null) {
          await tester.runAsync(() async {
            final loader = FontLoader(entry.key)
              ..addFont(
                File(path).readAsBytes().then((b) => ByteData.sublistView(b)),
              );
            if (entry.key == 'Roboto') {
              for (final env in [
                'HNUHOLE_UI_RENDER_LATIN_MEDIUM_FONT',
                'HNUHOLE_UI_RENDER_LATIN_BOLD_FONT',
              ]) {
                final extra = Platform.environment[env];
                if (extra != null) {
                  loader.addFont(
                    File(extra)
                        .readAsBytes()
                        .then((b) => ByteData.sublistView(b)),
                  );
                }
              }
            }
            await loader.load();
          });
        }
      }
      final previousShadows = debugDisableShadows;
      debugDisableShadows = false;
      try {
        final renderMedia = <String, MemoryImage>{};
        Future<void> createMedia(String id, Size size) async {
          await tester.runAsync(() async {
            final recorder = ui.PictureRecorder();
            final canvas = Canvas(recorder);
            final rect = Offset.zero & size;
            canvas.drawRect(
              rect,
              Paint()
                ..shader = const LinearGradient(
                  begin: Alignment.topLeft,
                  end: Alignment.bottomRight,
                  colors: [Color(0xFFDDEEE7), Color(0xFFB4CFD3)],
                ).createShader(rect),
            );
            canvas.drawCircle(
              Offset(size.width * .78, size.height * .23),
              size.shortestSide * .09,
              Paint()..color = const Color(0xFFFFF1CC),
            );
            canvas.drawPath(
              Path()
                ..moveTo(0, size.height * .67)
                ..quadraticBezierTo(
                  size.width * .4,
                  size.height * .28,
                  size.width,
                  size.height * .72,
                )
                ..lineTo(size.width, size.height)
                ..lineTo(0, size.height)
                ..close(),
              Paint()..color = const Color(0xFF6D9F8D),
            );
            canvas.drawRect(
              Rect.fromLTWH(
                size.width * .16,
                size.height * .43,
                size.width * .42,
                size.height * .32,
              ),
              Paint()..color = const Color(0xFFF9F7EC),
            );
            for (var i = 0; i < 4; i++) {
              canvas.drawRect(
                Rect.fromLTWH(
                  size.width * (.2 + i * .085),
                  size.height * .51,
                  size.width * .047,
                  size.height * .1,
                ),
                Paint()..color = const Color(0xFF517684),
              );
            }
            final picture = recorder.endRecording();
            final bitmap = await picture.toImage(
              size.width.toInt(),
              size.height.toInt(),
            );
            final bytes = await bitmap.toByteData(
              format: ui.ImageByteFormat.png,
            );
            renderMedia[id] = MemoryImage(bytes!.buffer.asUint8List());
            bitmap.dispose();
            picture.dispose();
          });
        }

        for (final size in [
          const Size(1600, 900),
          const Size(900, 1200),
          const Size(900, 900),
        ]) {
          await createMedia('${size.width}/${size.height}', size);
        }
        PostImage renderedMedia(String id, Size size) {
          final provider = renderMedia['${size.width}/${size.height}']!;
          return PostImage(
            thumbnail: provider,
            pixelSize: size,
            item: ViewerItem.provider(
              id: id,
              provider: provider,
              pixelSize: size,
              saveAllowed: false,
            ),
          );
        }

        final out = Directory(
          Platform.environment['HNUHOLE_FEED_RENDER_DIR'] ??
              'build/feed-ui-render',
        );
        await tester.runAsync(() => out.create(recursive: true));
        final f = await fixture.FeedFixture.create();
        addTearDown(f.dispose);
        final access = ValueNotifier(true);
        final boundary = GlobalKey();
        Future<void> capture(String name) async {
          await frames(tester);
          await tester.pumpAndSettle();
          final render =
              boundary.currentContext!.findRenderObject()!
                  as RenderRepaintBoundary;
          await tester.runAsync(() async {
            final png = await render.toImage(pixelRatio: 2);
            final bytes = await png.toByteData(format: ui.ImageByteFormat.png);
            await File('${out.path}/$name.png')
                .writeAsBytes(bytes!.buffer.asUint8List());
            png.dispose();
          });
          expect(tester.takeException(), isNull);
        }

        Future<void> mount({
          bool multi = false,
          Size size = const Size(1600, 900),
          Brightness brightness = Brightness.light,
        }) async {
          f.api.items = [fixture.post(images: multi ? 3 : 1)];
          await tester.pumpWidget(
            RepaintBoundary(
              key: boundary,
              child: fixture.screen(
                f,
                brightness: brightness,
                loader: (_) async => group([
                  renderedMedia('landscape', size),
                  if (multi) renderedMedia('portrait', const Size(900, 1200)),
                  if (multi) renderedMedia('square', const Size(900, 900)),
                ], access),
              ),
            ),
          );
          await tester.pumpAndSettle();
          await f.controller.loadTimeline();
          await tester.pumpAndSettle();
        }

        await mount();
        await capture('feed-light');
        await tester.tap(find.byKey(const ValueKey('feed-channel-picker')));
        await tester.pumpAndSettle();
        await capture('channels');
        await tester.tapAt(const Offset(4, 4));
        await tester.pumpAndSettle();
        await tester.tap(find.byKey(const ValueKey('feed-block-filter')));
        await tester.pumpAndSettle();
        await tester.tap(
          find.byKey(const ValueKey('block-channel:${fixture.otherChannelId}')),
        );
        await tester.pumpAndSettle();
        await capture('blocked');
        await tester.tapAt(const Offset(4, 4));
        await tester.pumpAndSettle();
        await tester.pumpWidget(const SizedBox());
        await mount(size: const Size(900, 1200));
        await capture('portrait');
        await tester.pumpWidget(const SizedBox());
        await mount(multi: true);
        await capture('carousel');
        await tester.pumpWidget(const SizedBox());
        await mount(brightness: Brightness.dark);
        await capture('feed-dark');
        await tester.pumpWidget(const SizedBox());
        access.dispose();
      } finally {
        debugDisableShadows = previousShadows;
      }
    },
  );
}
