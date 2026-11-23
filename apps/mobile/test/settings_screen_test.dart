import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_image_viewer/image_viewer.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';
import 'package:hnuhole_mobile/src/localization/app_localizations.dart';

import 'identity_test_support.dart';

class _DeniedGallery implements ViewerGalleryWriter {
  int requests = 0, writes = 0;
  @override
  Future<bool> hasAccess() async => false;
  @override
  Future<bool> requestAccess() async {
    requests++;
    return false;
  }

  @override
  Future<void> putImage(String path) async {
    writes++;
  }
}

class _PendingGallery implements ViewerGalleryWriter {
  final access = Completer<bool>();
  @override
  Future<bool> hasAccess() async => false;
  @override
  Future<bool> requestAccess() => access.future;
  @override
  Future<void> putImage(String path) async {}
}

class _ThrowingGallery implements ViewerGalleryWriter {
  @override
  Future<bool> hasAccess() async => false;
  @override
  Future<bool> requestAccess() async => throw StateError('permission failed');
  @override
  Future<void> putImage(String path) async {}
}

Finder _supportScrollable() => find.descendant(
  of: find.byKey(const ValueKey('settings-support-page')),
  matching: find.byType(Scrollable),
);

Future<void> _open(WidgetTester tester, String key) async {
  final scroll = find.descendant(
    of: find.byKey(const ValueKey('settings-list')),
    matching: find.byType(Scrollable),
  );
  tester.state<ScrollableState>(scroll).position.jumpTo(0);
  await tester.pump();
  final target = find.byKey(ValueKey('settings-$key'));
  await tester.scrollUntilVisible(target, 120, scrollable: scroll);
  // v6 分组行带副标题，行可能部分可见但中心在屏外；确保完整可见再点。
  await tester.ensureVisible(target);
  await tester.pumpAndSettle();
  await tester.tap(target);
  await tester.pumpAndSettle();
}

Widget _app(AuthPresentationController c, {SettingsScreen? settings}) =>
    AnimatedBuilder(
      animation: c,
      builder: (context, _) => MaterialApp(
        locale: c.language.locale,
        supportedLocales: appSupportedLocales,
        localizationsDelegates: appLocalizationDelegates,
        builder: (context, child) =>
            AuthPresentationScope(controller: c, child: child!),
        home:
            settings ??
            SettingsScreen(
              onManageIdentity: () {},
              onManageSecurity: () {},
              onLogout: () {},
            ),
      ),
    );

void main() {
  testWidgets(
    'settings press cancellation and keyboard activation keep one route',
    (tester) async {
      final c = AuthPresentationController();
      var activations = 0;
      await tester.pumpWidget(
        _app(
          c,
          settings: SettingsScreen(
            onManageIdentity: () {},
            onManageSecurity: () => activations++,
            onLogout: () {},
          ),
        ),
      );
      await tester.pumpAndSettle();
      final devices = find.byKey(const ValueKey('settings-devices'));
      final point = tester.getCenter(devices);
      final gesture = await tester.startGesture(point);
      await tester.pump(const Duration(milliseconds: 80));
      expect(activations, 0);
      await gesture.moveBy(const Offset(0, -100));
      await gesture.up();
      await tester.pumpAndSettle();
      expect(activations, 0);
      await tester.tap(devices);
      await tester.pumpAndSettle();
      expect(activations, 1);
      // A pointer activation leaves the entry focusable for native keyboard use.
      for (var step = 0; step < 12; step++) {
        final focused = FocusManager.instance.primaryFocus?.context;
        if (focused != null &&
            find
                .ancestor(
                  of: find.byElementPredicate((element) => element == focused),
                  matching: devices,
                )
                .evaluate()
                .isNotEmpty) {
          break;
        }
        await tester.sendKeyEvent(LogicalKeyboardKey.tab);
        await tester.pump();
      }
      final focusContext = FocusManager.instance.primaryFocus?.context;
      expect(focusContext, isNotNull);
      expect(
        find.ancestor(
          of: find.byElementPredicate((element) => element == focusContext),
          matching: devices,
        ),
        findsOneWidget,
      );
      await tester.sendKeyEvent(LogicalKeyboardKey.enter);
      await tester.pumpAndSettle();
      expect(activations, 2);
      await tester.pumpWidget(const SizedBox());
      c.dispose();
    },
  );

  testWidgets(
    'support enlarges original and reports denied save without success',
    (tester) async {
      final c = AuthPresentationController();
      final gallery = _DeniedGallery();
      await tester.pumpWidget(
        _app(
          c,
          settings: SettingsScreen(
            onManageIdentity: () {},
            onManageSecurity: () {},
            onLogout: () {},
            supportGalleryWriter: gallery,
          ),
        ),
      );
      await tester.pumpAndSettle();
      await _open(tester, 'support');
      final entry = find.byKey(const ValueKey('settings-support-code-entry'));
      expect(find.byKey(const ValueKey('settings-support-code')), findsNothing);
      await tester.ensureVisible(entry);
      await tester.pumpAndSettle();
      await tester.tap(entry);
      await tester.pumpAndSettle();
      final code = find.byKey(const ValueKey('settings-support-code'));
      expect(code, findsOneWidget);
      expect(
        (tester.widget<Image>(code).image as AssetImage).assetName,
        'assets/support/alipay-receive.jpg',
      );
      await tester.binding.handlePopRoute();
      await tester.pumpAndSettle();
      await tester.scrollUntilVisible(
        find.byKey(const ValueKey('settings-support-save')),
        180,
        scrollable: find.descendant(
          of: find.byKey(const ValueKey('settings-support-page')),
          matching: find.byType(Scrollable),
        ),
      );
      await tester.tap(find.byKey(const ValueKey('settings-support-save')));
      await tester.pumpAndSettle();
      expect(gallery.requests, 1);
      expect(gallery.writes, 0);
      expect(find.text('请允许访问相册后重试'), findsOneWidget);
      expect(find.text('收款码已保存到相册'), findsNothing);
      await tester.pumpWidget(const SizedBox());
      c.dispose();
    },
  );
  testWidgets('settings ends with logout and no account-security entry', (
    tester,
  ) async {
    final c = AuthPresentationController();
    var devices = 0, logout = 0, closure = 0, identities = 0, blacklist = 0;
    await tester.pumpWidget(
      _app(
        c,
        settings: SettingsScreen(
          onManageIdentity: () => identities++,
          onManageSecurity: () => devices++,
          onLogout: () => logout++,
          onCloseAccount: () => closure++,
          onBlacklist: () => blacklist++,
        ),
      ),
    );
    await tester.pumpAndSettle();
    for (final key in ['devices', 'closure', 'identity', 'blacklist']) {
      await _open(tester, key);
    }
    await _open(tester, 'logout');
    expect(logout, 0);
    expect(
      find.byKey(const ValueKey('settings-logout-confirmation')),
      findsOneWidget,
    );
    await tester.tap(find.byKey(const ValueKey('settings-logout-confirm')));
    await tester.pumpAndSettle();
    expect([devices, logout, closure, identities, blacklist], [1, 1, 1, 1, 1]);
    expect(find.byKey(const ValueKey('settings-account')), findsNothing);
    // 分组结构（原型）：分组标签「账号与安全」常驻，但条目行不因 onLogout 缺省出现。
    expect(find.text('账号与安全'), findsOneWidget);
    expect(find.text('退出登录'), findsOneWidget);
    expect(find.byKey(const ValueKey('settings-logout')), findsOneWidget);
    expect(find.byKey(const ValueKey('settings-notifications')), findsNothing);
    expect(find.byType(TextField), findsNothing);
    await tester.pumpWidget(const SizedBox());
    c.dispose();
  });
  testWidgets(
    'four languages apply without replacing the route and persist locally',
    (tester) async {
      final vault = IdentityTestVault();
      final c = AuthPresentationController(preferences: vault);
      await tester.pumpWidget(_app(c));
      await tester.pumpAndSettle();
      await _open(tester, 'language');
      final route = ModalRoute.of(
        tester.element(find.byKey(const ValueKey('settings-language-page'))),
      );
      for (final language in AppLanguage.values) {
        await tester.tap(
          find.byKey(ValueKey('settings-language-${language.code}')),
        );
        await tester.pumpAndSettle();
        expect(c.language, language);
        expect(
          ModalRoute.of(
            tester.element(
              find.byKey(const ValueKey('settings-language-page')),
            ),
          ),
          same(route),
        );
        expect(
          Localizations.localeOf(
            tester.element(
              find.byKey(const ValueKey('settings-language-page')),
            ),
          ),
          language.locale,
        );
        await c.preferencesSaved;
        final restored = AuthPresentationController(preferences: vault);
        await restored.load();
        expect(restored.language, language);
        restored.dispose();
      }
      await tester.pumpWidget(const SizedBox());
      c.dispose();
    },
  );
  testWidgets(
    'feedback only lists and copies the supplied email without sending',
    (tester) async {
      final c = AuthPresentationController();
      String? copied;
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        (call) async {
          if (call.method == 'Clipboard.setData') {
            copied = (call.arguments as Map)['text'] as String;
          }
          return null;
        },
      );
      addTearDown(
        () => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
          SystemChannels.platform,
          null,
        ),
      );
      await tester.pumpWidget(_app(c));
      await tester.pumpAndSettle();
      await _open(tester, 'feedback');
      expect(find.text('zewbby666@gmail.com'), findsOneWidget);
      expect(find.byType(TextFormField), findsNothing);
      expect(
        find.byKey(const ValueKey('settings-feedback-submit')),
        findsNothing,
      );
      await tester.tap(find.byKey(const ValueKey('settings-feedback-copy')));
      await tester.pumpAndSettle();
      expect(copied, 'zewbby666@gmail.com');
      expect(find.text('邮箱已复制'), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
      c.dispose();
    },
  );
  testWidgets(
    'support uses original asset and the requested caption in four languages',
    (tester) async {
      final c = AuthPresentationController();
      await tester.pumpWidget(_app(c));
      await tester.pumpAndSettle();
      await _open(tester, 'support');
      expect(
        find.byKey(const ValueKey('settings-support-page')),
        findsOneWidget,
      );
      expect(find.byKey(const ValueKey('settings-support-code')), findsNothing);
      await tester.tap(
        find.byKey(const ValueKey('settings-support-code-entry')),
      );
      await tester.pumpAndSettle();
      final image = tester.widget<Image>(
        find.byKey(const ValueKey('settings-support-code')),
      );
      expect(
        (image.image as AssetImage).assetName,
        'assets/support/alipay-receive.jpg',
      );
      await tester.binding.handlePopRoute();
      await tester.pumpAndSettle();
      for (final language in AppLanguage.values) {
        c.setLanguage(language);
        await tester.pumpAndSettle();
        await tester.scrollUntilVisible(
          find.byKey(const ValueKey('settings-support-save')),
          180,
          scrollable: find.descendant(
            of: find.byKey(const ValueKey('settings-support-page')),
            matching: find.byType(Scrollable),
          ),
        );
        expect(
          tester
              .widget<Text>(
                find.byKey(const ValueKey('settings-support-purpose')),
              )
              .data,
          translateUi('您的支持将全数用于开发与维护SpeechFUL项目', language),
        );
        expect(
          find.byKey(const ValueKey('settings-support-save')),
          findsOneWidget,
        );
      }
      await tester.pumpWidget(const SizedBox());
      c.dispose();
    },
  );
  testWidgets('support page exposes card sections and a primary save action', (
    tester,
  ) async {
    final c = AuthPresentationController();
    await tester.pumpWidget(_app(c));
    await tester.pumpAndSettle();
    await _open(tester, 'support');
    expect(
      find.byKey(const ValueKey('settings-support-intro')),
      findsOneWidget,
    );
    final codeCard = find.byKey(const ValueKey('settings-support-code-card'));
    expect(codeCard, findsOneWidget);
    expect(
      find.descendant(
        of: codeCard,
        matching: find.byKey(const ValueKey('settings-support-code-entry')),
      ),
      findsOneWidget,
    );
    expect(find.byKey(const ValueKey('settings-support-code')), findsNothing);
    await tester.tap(find.byKey(const ValueKey('settings-support-code-entry')));
    await tester.pumpAndSettle();
    // 收款码受限最大宽度，宽屏上不被无限放大。
    expect(
      tester.getSize(find.byKey(const ValueKey('settings-support-code'))).width,
      lessThanOrEqualTo(300),
    );
    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    final save = find.byKey(const ValueKey('settings-support-save'));
    await tester.scrollUntilVisible(
      save,
      160,
      scrollable: _supportScrollable(),
    );
    await tester.ensureVisible(save);
    await tester.pumpAndSettle();
    expect(
      find.byKey(const ValueKey('settings-support-purpose-card')),
      findsOneWidget,
    );
    expect(
      find.byKey(const ValueKey('settings-support-instructions')),
      findsOneWidget,
    );
    final purpose = tester.widget<Text>(
      find.byKey(const ValueKey('settings-support-purpose')),
    );
    expect(purpose.maxLines, isNull);
    expect(purpose.overflow, isNull);
    final button = tester.widget<FilledButton>(save);
    expect(button.onPressed, isNotNull);
    expect(
      find.descendant(of: save, matching: find.text('保存收款码')),
      findsOneWidget,
    );
    expect(find.byKey(const ValueKey('settings-support-status')), findsNothing);
    await tester.pumpWidget(const SizedBox());
    c.dispose();
  });
  testWidgets(
    'support save disables while busy and announces a denied result',
    (tester) async {
      final c = AuthPresentationController();
      final gallery = _PendingGallery();
      await tester.pumpWidget(
        _app(
          c,
          settings: SettingsScreen(
            onManageIdentity: () {},
            onManageSecurity: () {},
            onLogout: () {},
            supportGalleryWriter: gallery,
          ),
        ),
      );
      await tester.pumpAndSettle();
      await _open(tester, 'support');
      final save = find.byKey(const ValueKey('settings-support-save'));
      await tester.scrollUntilVisible(
        save,
        160,
        scrollable: _supportScrollable(),
      );
      await tester.ensureVisible(save);
      await tester.pumpAndSettle();
      await tester.tap(save);
      await tester.pump();
      expect(tester.widget<FilledButton>(save).onPressed, isNull);
      expect(find.text('保存中…'), findsOneWidget);
      expect(
        find.byKey(const ValueKey('settings-support-status')),
        findsNothing,
      );
      gallery.access.complete(false);
      await tester.pumpAndSettle();
      expect(tester.widget<FilledButton>(save).onPressed, isNotNull);
      expect(find.text('保存收款码'), findsOneWidget);
      expect(find.text('保存中…'), findsNothing);
      final status = find.byKey(const ValueKey('settings-support-status'));
      expect(status, findsOneWidget);
      expect(tester.widget<Semantics>(status).properties.liveRegion, isTrue);
      expect(find.text('请允许访问相册后重试'), findsOneWidget);
      expect(find.text('收款码已保存到相册'), findsNothing);
      await tester.pumpWidget(const SizedBox());
      c.dispose();
    },
  );
  testWidgets('support save reports an unspecified failure without success', (
    tester,
  ) async {
    final c = AuthPresentationController();
    await tester.pumpWidget(
      _app(
        c,
        settings: SettingsScreen(
          onManageIdentity: () {},
          onManageSecurity: () {},
          onLogout: () {},
          supportGalleryWriter: _ThrowingGallery(),
        ),
      ),
    );
    await tester.pumpAndSettle();
    await _open(tester, 'support');
    final save = find.byKey(const ValueKey('settings-support-save'));
    await tester.scrollUntilVisible(
      save,
      160,
      scrollable: _supportScrollable(),
    );
    await tester.ensureVisible(save);
    await tester.pumpAndSettle();
    await tester.tap(save);
    await tester.pumpAndSettle();
    expect(find.text('暂时无法保存，请重试'), findsOneWidget);
    expect(find.text('收款码已保存到相册'), findsNothing);
    expect(tester.widget<FilledButton>(save).onPressed, isNotNull);
    expect(
      tester
          .widget<Semantics>(
            find.byKey(const ValueKey('settings-support-status')),
          )
          .properties
          .liveRegion,
      isTrue,
    );
    await tester.pumpWidget(const SizedBox());
    c.dispose();
  });
  testWidgets(
    'support layout stays scrollable on narrow large-text and landscape screens',
    (tester) async {
      final c = AuthPresentationController();
      addTearDown(c.dispose);
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      addTearDown(tester.platformDispatcher.clearTextScaleFactorTestValue);
      const scenarios = [
        (Size(320, 740), 1.8),
        (Size(740, 360), 1.3),
        (Size(1000, 800), 1.0),
      ];
      for (final (size, scale) in scenarios) {
        tester.view.physicalSize = size;
        tester.view.devicePixelRatio = 1;
        tester.platformDispatcher.textScaleFactorTestValue = scale;
        await tester.pumpWidget(const SizedBox());
        await tester.pumpWidget(_app(c));
        await tester.pumpAndSettle();
        await _open(tester, 'support');
        expect(
          find.byKey(const ValueKey('settings-support-code-card')),
          findsOneWidget,
        );
        expect(
          find.byKey(const ValueKey('settings-support-code')),
          findsNothing,
        );
        await tester.tap(
          find.byKey(const ValueKey('settings-support-code-entry')),
        );
        await tester.pumpAndSettle();
        expect(
          tester
              .getSize(find.byKey(const ValueKey('settings-support-code')))
              .width,
          lessThanOrEqualTo(300),
          reason: '${size.width}x${size.height} @ $scale',
        );
        await tester.binding.handlePopRoute();
        await tester.pumpAndSettle();
        expect(
          tester.takeException(),
          isNull,
          reason: '${size.width}x${size.height} @ $scale',
        );
        final save = find.byKey(const ValueKey('settings-support-save'));
        await tester.scrollUntilVisible(
          save,
          200,
          scrollable: _supportScrollable(),
        );
        await tester.ensureVisible(save);
        await tester.pumpAndSettle();
        expect(
          tester.takeException(),
          isNull,
          reason: '${size.width}x${size.height} @ $scale',
        );
        expect(save, findsOneWidget);
        final purpose = tester.widget<Text>(
          find.byKey(const ValueKey('settings-support-purpose')),
        );
        expect(purpose.maxLines, isNull);
        expect(purpose.overflow, isNull);
        await tester.binding.handlePopRoute();
        await tester.pumpAndSettle();
      }
      await tester.pumpWidget(const SizedBox());
    },
  );
}
