import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';
import 'package:hnuhole_mobile/src/auth/auth_wordmark.dart';
import 'package:hnuhole_mobile/src/auth/device_management_controller.dart';
import 'package:hnuhole_mobile/src/auth/device_management_screen.dart';

import 'identity_test_support.dart';
import 'login_home_test_support.dart';
import 'security_settings_test_support.dart';

void expectHome(
  WidgetTester tester,
  HomeTestFixture fixture, {
  String? channelId,
  String title = '推荐',
}) {
  expect(find.byType(FeedScreen), findsOneWidget);
  expect(find.byType(EntryScreen), findsNothing);
  expect(find.byType(AuthScreen), findsNothing);
  expect(fixture.posts.timelineChannelId, channelId);
  expect(
    find.descendant(
      of: find.byKey(const ValueKey('feed-recommended')),
      matching: find.text(title),
    ),
    findsOneWidget,
  );
  final context = tester.element(find.byType(FeedScreen));
  expect(ModalRoute.of(context)!.isFirst, isTrue);
  expect(Navigator.of(context).canPop(), isFalse);
  final home = find
      .ancestor(
        of: find.byKey(const ValueKey('destination:home')),
        matching: find.byType(Semantics),
      )
      .first;
  expect(tester.widget<Semantics>(home).properties.selected, isTrue);
}

Future<void> pickChannel(WidgetTester tester, String id) async {
  await tester.tap(find.byKey(const ValueKey('feed-channel-picker')));
  await tester.pumpAndSettle();
  final choice = find.byKey(ValueKey('choose-channel:$id'));
  await tester.scrollUntilVisible(
    choice,
    150,
    scrollable: find.descendant(
      of: find.byKey(const ValueKey('channel-options')),
      matching: find.byType(Scrollable),
    ),
  );
  await tester.tap(choice);
  await tester.pumpAndSettle();
}

Future<void> disposeApp(WidgetTester tester) async {
  await tester.pumpWidget(const SizedBox.shrink());
  await tester.pump();
}

Future<void> openLogoutSetting(WidgetTester tester) async {
  await openSettingsFromHome(tester);
  await tester.scrollUntilVisible(
    find.byKey(const ValueKey('settings-logout')),
    100,
    scrollable: find.descendant(
      of: find.byKey(const ValueKey('settings-list')),
      matching: find.byType(Scrollable),
    ),
  );
  await tester.ensureVisible(find.byKey(const ValueKey('settings-logout')));
  await tester.pump();
}

Future<void> confirmSettingsLogout(WidgetTester tester) async {
  expect(
    find.byKey(const ValueKey('settings-logout-confirmation')),
    findsOneWidget,
  );
  await tester.tap(find.byKey(const ValueKey('settings-logout-confirm')));
  await tester.pumpAndSettle();
}

void main() {
  testWidgets(
    'restoration keeps the complete welcome branding while directory is pending',
    (tester) async {
      tester.view.padding = const FakeViewPadding(top: 24, bottom: 34);
      addTearDown(tester.view.resetPadding);
      final f = HomeTestFixture();
      f.channels.wait = Completer<ChannelDirectoryResult>();
      await tester.pumpWidget(await f.create());
      await tester.pumpAndSettle();
      expect(find.byType(AuthWelcomeScreen), findsOneWidget);
      expect(
        tester
            .widget<AuthWelcomeScreen>(find.byType(AuthWelcomeScreen))
            .showControls,
        isFalse,
      );
      expect(find.byType(CircularProgressIndicator), findsNothing);
      expect(find.byType(FeedScreen), findsNothing);
      expect(f.postApi.calls, isEmpty);
      final brand = tester.getRect(find.byType(AuthWordmark));
      await tester.pump(const Duration(seconds: 10));
      expect(tester.binding.transientCallbackCount, 0);
      expect(tester.getRect(find.byType(AuthWordmark)), brand);
      f.channels.wait!.complete(
        await IdentityTestChannels().loadChannels(
          sessionToken: identityTestToken,
        ),
      );
      await tester.pumpAndSettle();
      expectHome(tester, f);
      expect(find.byType(CircularProgressIndicator), findsNothing);
      await disposeApp(tester);
    },
  );
  testWidgets('cold start never presents the suspended-session error page', (
    tester,
  ) async {
    final f = HomeTestFixture();
    await tester.pumpWidget(await f.create());
    expect(find.text('会话暂时无法确认，请重试。'), findsNothing);
    expect(find.text('重新核对会话'), findsNothing);
    await tester.pumpAndSettle();
    expectHome(tester, f);
    await disposeApp(tester);
  });

  testWidgets('password login goes directly to recommended root home', (
    tester,
  ) async {
    final f = HomeTestFixture();
    await tester.pumpWidget(await f.create(signedIn: false));
    await tester.pumpAndSettle();
    expect(find.byType(FeedScreen), findsNothing);
    await tester.tap(find.byKey(const ValueKey('auth-start')));
    await tester.pumpAndSettle();
    await tester.enterText(find.byType(TextFormField).at(0), 'device_a');
    await tester.enterText(
      find.byType(TextFormField).at(1),
      'valid-password-123',
    );
    await tester.tap(find.byKey(const ValueKey('login-submit')));
    await tester.pumpAndSettle();
    expect(f.sessionApi.logins, 1);
    expectHome(tester, f);
    expect(f.postApi.calls, contains('feed:recommended:first:20'));
    await disposeApp(tester);
  });

  testWidgets('login stays on its form until the home directory is ready', (
    tester,
  ) async {
    final f = HomeTestFixture();
    f.channels.wait = Completer<ChannelDirectoryResult>();
    await tester.pumpWidget(await f.create(signedIn: false));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('auth-start')));
    await tester.pumpAndSettle();
    await tester.enterText(find.byType(TextFormField).at(0), 'device_a');
    await tester.enterText(
      find.byType(TextFormField).at(1),
      'valid-password-123',
    );
    await tester.tap(find.byKey(const ValueKey('login-submit')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 500));

    expect(f.sessions.isAuthenticated, isTrue);
    expect(find.byType(AuthScreen), findsOneWidget);
    expect(find.byType(AuthWelcomeScreen), findsNothing);
    expect(find.text('账号已登录'), findsNothing);
    expect(find.byType(FeedScreen), findsNothing);
    expect(
      tester
          .widget<FilledButton>(find.byKey(const ValueKey('login-submit')))
          .onPressed,
      isNull,
    );

    f.channels.wait!.complete(
      await IdentityTestChannels().loadChannels(
        sessionToken: identityTestOtherToken,
      ),
    );
    await tester.pumpAndSettle();
    expectHome(tester, f);
    await disposeApp(tester);
  });

  testWidgets('settings logout confirms before reaching stable welcome', (
    tester,
  ) async {
    final f = HomeTestFixture();
    await tester.pumpWidget(await f.create());
    await tester.pumpAndSettle();
    final staleAuth = tester
        .widget<FeedScreen>(find.byType(FeedScreen))
        .onAuthenticationRequired;
    await openLogoutSetting(tester);
    await tester.tap(find.byKey(const ValueKey('settings-logout')));
    await tester.pumpAndSettle();

    expect(f.sessions.isAuthenticated, isTrue);
    expect(f.store.current!.session, isNotNull);
    expect(find.byType(AuthWelcomeScreen), findsNothing);
    expect(
      find.byKey(const ValueKey('settings-logout-confirmation')),
      findsOneWidget,
    );
    await tester.tap(find.byKey(const ValueKey('settings-logout-cancel')));
    await tester.pumpAndSettle();
    expect(f.sessions.isAuthenticated, isTrue);
    expect(find.byType(SettingsScreen), findsOneWidget);
    expect(
      find.byKey(const ValueKey('settings-logout-confirmation')),
      findsNothing,
    );

    await tester.tap(find.byKey(const ValueKey('settings-logout')));
    await tester.pumpAndSettle();
    expect(f.sessions.isAuthenticated, isTrue);
    await confirmSettingsLogout(tester);

    expect(f.sessions.status, AuthStatus.signedOut);
    expect(f.store.current!.session, isNull);
    expect(find.byType(AuthWelcomeScreen), findsOneWidget);
    expect(find.byKey(const ValueKey('auth-start')), findsOneWidget);
    expect(find.byType(AuthScreen, skipOffstage: false), findsNothing);
    expect(find.byType(FeedScreen, skipOffstage: false), findsNothing);
    expect(find.byType(SettingsScreen, skipOffstage: false), findsNothing);
    final context = tester.element(find.byType(AuthWelcomeScreen));
    expect(ModalRoute.of(context)!.isFirst, isTrue);
    expect(Navigator.of(context).canPop(), isFalse);
    staleAuth();
    await tester.pumpAndSettle();
    await tester.pump(const Duration(seconds: 2));
    expect(find.byType(AuthWelcomeScreen), findsOneWidget);
    expect(find.byType(AuthScreen, skipOffstage: false), findsNothing);
    await disposeApp(tester);
  });

  testWidgets('logout never presents the entry failure dialog', (tester) async {
    final f = HomeTestFixture();
    await tester.pumpWidget(await f.create());
    await tester.pumpAndSettle();
    await openLogoutSetting(tester);
    await tester.tap(find.byKey(const ValueKey('settings-logout')));
    await tester.pumpAndSettle();
    final finishRead = Completer<void>();
    f.vault.nextReadWait = finishRead;
    await tester.tap(find.byKey(const ValueKey('settings-logout-confirm')));
    await tester.pumpAndSettle();

    expect(f.vault.readBlocked.isCompleted, isTrue);
    expect(f.sessions.busy, isTrue);
    expect(
      find.byKey(const ValueKey('entry-unavailable-dialog')),
      findsNothing,
    );
    finishRead.complete();
    await tester.pumpAndSettle();
    expect(f.sessions.status, AuthStatus.signedOut);
    expect(find.byType(AuthWelcomeScreen), findsOneWidget);
    expect(
      find.byKey(const ValueKey('entry-unavailable-dialog')),
      findsNothing,
    );
    await disposeApp(tester);
  });

  testWidgets('device logout clears nested protected routes to welcome', (
    tester,
  ) async {
    final f = HomeTestFixture();
    await tester.pumpWidget(await f.create());
    await tester.pumpAndSettle();
    await tester.tap(find.text('今天的校园'));
    await tester.pumpAndSettle();
    final navigator = Navigator.of(
      tester.element(find.byType(PostDetailScreen)),
    );
    final controller = DeviceManagementController(
      api: SecuritySettingsApi(),
      store: f.store,
      sessions: f.sessions,
    );
    unawaited(
      navigator.push(
        MaterialPageRoute<void>(
          builder: (_) => DeviceManagementScreen(
            controller: controller,
            sessions: f.sessions,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('退出本机'));
    await tester.pumpAndSettle();

    expect(f.sessions.status, AuthStatus.signedOut);
    expect(find.byType(AuthWelcomeScreen), findsOneWidget);
    expect(find.byType(AuthScreen, skipOffstage: false), findsNothing);
    expect(
      find.byType(DeviceManagementScreen, skipOffstage: false),
      findsNothing,
    );
    expect(find.byType(PostDetailScreen, skipOffstage: false), findsNothing);
    expect(navigator.canPop(), isFalse);
    await disposeApp(tester);
    controller.dispose();
  });

  testWidgets('revocation retry does not navigate away from welcome', (
    tester,
  ) async {
    final f = HomeTestFixture();
    f.sessionApi.revocationFailure = const AuthFailure(
      kind: AuthFailureKind.timeout,
    );
    await tester.pumpWidget(await f.create());
    await tester.pumpAndSettle();
    await openLogoutSetting(tester);
    await tester.tap(find.byKey(const ValueKey('settings-logout')));
    await tester.pumpAndSettle();
    await confirmSettingsLogout(tester);
    expect(f.sessions.status, AuthStatus.signedOut);
    expect(f.sessions.hasPendingLogout, isTrue);
    expect(find.byType(AuthWelcomeScreen), findsOneWidget);
    f.sessionApi.revocationFailure = null;
    await f.sessions.retryPendingLogout();
    await tester.pumpAndSettle();
    expect(f.sessions.hasPendingLogout, isFalse);
    expect(find.byType(AuthWelcomeScreen), findsOneWidget);
    expect(find.byType(AuthScreen, skipOffstage: false), findsNothing);
    await disposeApp(tester);
  });

  testWidgets('logout storage failure retains the error screen', (
    tester,
  ) async {
    final f = HomeTestFixture();
    await tester.pumpWidget(await f.create());
    await tester.pumpAndSettle();
    f.vault.failContaining = '"tokenDigest"';
    await openLogoutSetting(tester);
    await tester.tap(find.byKey(const ValueKey('settings-logout')));
    await tester.pumpAndSettle();
    await confirmSettingsLogout(tester);
    expect(f.sessions.status, AuthStatus.storageFailure);
    expect(find.byType(AuthScreen), findsOneWidget);
    expect(find.byKey(const ValueKey('login-submit')), findsNothing);
    expect(find.byType(FeedScreen, skipOffstage: false), findsNothing);
    await disposeApp(tester);
  });

  testWidgets(
    'restored session opens recommended home with no selection page',
    (tester) async {
      final f = HomeTestFixture();
      await tester.pumpWidget(await f.create());
      await tester.pumpAndSettle();
      expect(f.sessionApi.logins, 0);
      expectHome(tester, f);
      // Returning from a post cannot reveal a former selection page.
      await tester.tap(find.text('今天的校园'));
      await tester.pumpAndSettle();
      expect(find.byType(PostDetailScreen), findsOneWidget);
      Navigator.of(tester.element(find.byType(PostDetailScreen))).pop();
      await tester.pumpAndSettle();
      expectHome(tester, f);
      await disposeApp(tester);
    },
  );

  for (final sameAccount in [false, true]) {
    testWidgets(
      'login retains only this account channel within app run; sameAccount=$sameAccount',
      (tester) async {
        final f = HomeTestFixture();
        await tester.pumpWidget(await f.create());
        await tester.pumpAndSettle();
        final oldHome = tester.state(find.byType(FeedScreen));
        final channel = f.directory.channels.last.id;
        await pickChannel(tester, channel);
        expect(f.posts.timelineChannelId, channel);
        await tester.tap(find.byKey(const ValueKey('destination:personal')));
        await tester.pumpAndSettle();
        await f.loginB(
          tester,
          account: sameAccount
              ? identityTestAccount
              : '00000000-0000-0000-0000-000000000002',
          openPersonal: false,
          expectedChannelId: sameAccount ? channel : null,
        );
        expectHome(
          tester,
          f,
          channelId: sameAccount ? channel : null,
          title: sameAccount ? f.directory.channels.last.name : '推荐',
        );
        expect(find.byType(SettingsScreen, skipOffstage: false), findsNothing);
        expect(
          find.byType(PersonalPostsScreen, skipOffstage: false),
          findsNothing,
        );
        expect(
          identical(oldHome, tester.state(find.byType(FeedScreen))),
          isFalse,
        );
        await disposeApp(tester);
      },
    );
  }

  testWidgets(
    'home button preserves selected channel, loaded data and root route',
    (tester) async {
      final f = HomeTestFixture();
      await tester.pumpWidget(await f.create());
      await tester.pumpAndSettle();
      final channel = f.directory.channels.first;
      await pickChannel(tester, channel.id);
      final home = tester.state(find.byType(FeedScreen));
      final calls = List.of(f.postApi.calls);
      for (var i = 0; i < 2; i++) {
        await tester.tap(find.byKey(const ValueKey('destination:home')));
        await tester.pumpAndSettle();
        expectHome(tester, f, channelId: channel.id, title: channel.name);
        expect(identical(home, tester.state(find.byType(FeedScreen))), isTrue);
        expect(f.postApi.calls, calls);
      }
      await disposeApp(tester);
    },
  );

  for (final fromDetail in [true, false]) {
    testWidgets(
      'return from another page preserves selected channel; detail=$fromDetail',
      (tester) async {
        final f = HomeTestFixture();
        await tester.pumpWidget(await f.create());
        await tester.pumpAndSettle();
        final channel = f.directory.channels.first;
        await pickChannel(tester, channel.id);
        final home = tester.state(find.byType(FeedScreen));
        if (fromDetail) {
          await tester.tap(find.text('今天的校园'));
          await tester.pumpAndSettle();
          Navigator.of(tester.element(find.byType(PostDetailScreen))).pop();
        } else {
          await tester.tap(find.byKey(const ValueKey('destination:personal')));
          await tester.pumpAndSettle();
          await tester.tap(find.byTooltip('首页'));
        }
        await tester.pumpAndSettle();
        expectHome(tester, f, channelId: channel.id, title: channel.name);
        expect(identical(home, tester.state(find.byType(FeedScreen))), isTrue);
        await disposeApp(tester);
      },
    );
  }

  testWidgets(
    'new app run starts recommended after the prior run selected a channel',
    (tester) async {
      final first = HomeTestFixture();
      await tester.pumpWidget(await first.create());
      await tester.pumpAndSettle();
      await pickChannel(tester, first.directory.channels.first.id);
      await disposeApp(tester);
      final next = HomeTestFixture();
      await tester.pumpWidget(await next.create());
      await tester.pumpAndSettle();
      expectHome(tester, next);
      await disposeApp(tester);
    },
  );

  testWidgets('same-token deadline renewal preserves channel and home state', (
    tester,
  ) async {
    final f = HomeTestFixture();
    await tester.pumpWidget(await f.create());
    await tester.pumpAndSettle();
    final channel = f.directory.channels.first.id;
    await pickChannel(tester, channel);
    final home = tester.state(find.byType(FeedScreen));
    await f.sessions.persistSessionMetadata(
      identityTestToken,
      identityTestRenewed,
      () => true,
    );
    await tester.pumpAndSettle();
    expect(f.posts.timelineChannelId, channel);
    expect(identical(home, tester.state(find.byType(FeedScreen))), isTrue);
    await disposeApp(tester);
  });

  testWidgets('post loading failure keeps recommended home and allows retry', (
    tester,
  ) async {
    final f = HomeTestFixture();
    f.postApi.timelineFailure = const PostFailure(
      kind: PostFailureKind.transport,
    );
    await tester.pumpWidget(await f.create());
    await tester.pumpAndSettle();
    expectHome(tester, f);
    expect(find.text('重新加载'), findsOneWidget);
    await tester.tap(find.text('重新加载'));
    await tester.pumpAndSettle();
    expectHome(tester, f);
    expect(find.text('今天的校园'), findsOneWidget);
    await disposeApp(tester);
  });
}
