import 'dart:async';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';

import 'auth_flows_test.dart' as f;
import 'email_reset_test.dart' as reset;

class _Api extends reset.ResetApi implements RegistrationResultApi {
  RegistrationResult result = RegistrationResult.pending;
  AuthFailure? resultFailure;
  Completer<RegistrationResult>? response;
  final queries = <Map<String, String>>[];
  @override
  Future<CurrentSession> currentSession(String sessionToken) async =>
      CurrentSession(
        accountId: f.session.accountId,
        username: 'private_user',
        expiresAt: DateTime.now().toUtc().add(const Duration(days: 30)),
      );
  @override
  Future<RegistrationResult> registrationResult({
    required String intentId,
    required String idempotencyKey,
  }) async {
    queries.add({'id': intentId, 'key': idempotencyKey});
    if (resultFailure != null) throw resultFailure!;
    return response != null ? response!.future : result;
  }
}

class _Harness {
  final vault = f.Vault(), api = _Api();
  late final store = AuthStore(vault);
  int authority = 0;
  AuthFlows make() => AuthFlows(
    api: api,
    store: store,
    legacyRecoveryEnabled: false,
    clock: () => f.now,
    sessionAuthorityVersion: () => authority,
    clearCommunityAccess: () {},
    acceptRegistrationSession: (session) => store.update(
      (s) => s.copyWith(
        session: SessionRecord(
          token: session.sessionToken,
          accountId: session.accountId,
          expiresAt: session.expiresAt,
        ),
      ),
    ),
  );

  Future<void> seedUnknown() async {
    final source = f.Harness();
    await source.eligibility();
    await source.flows.createRegistration('private_user', 'Independent123!');
    source.flows.hideRecoveryCode();
    source.api.onRegistration = () async =>
        throw const AuthFailure(kind: AuthFailureKind.timeout);
    await source.flows.commitRegistration('confirmation');
    final state = await source.store.read();
    await store.update((_) => state);
    source.flows.dispose();
  }

  Future<void> seedEligibility() async {
    final source = f.Harness();
    await source.eligibility();
    await store.update((_) => source.store.current!);
    api.ticket = source.api.ticket;
    source.flows.dispose();
  }
}

class _Channels implements ChannelRepository {
  @override
  Future<ChannelDirectoryResult> loadChannels({
    required String sessionToken,
  }) async => ChannelDirectoryResult(
    expiresAt: DateTime.now().toUtc().add(const Duration(days: 30)),
    channels: [
      for (var i = 0; i < ChannelDirectory.requiredCodes.length; i++)
        Channel(
          id: 'channel-$i',
          code: ChannelDirectory.requiredCodes[i],
          name: ChannelDirectory.requiredCodes[i],
          initiallyVisible: i < 5,
          displayOrder: i,
        ),
    ],
  );
}

class _Screen {
  _Screen(this.h) {
    sessions = AuthSessionController(
      api: h.api,
      store: h.store,
      directory: directory,
    );
    flow = AuthFlows(
      api: h.api,
      store: h.store,
      legacyRecoveryEnabled: false,
      clock: () => f.now,
      sessionAuthorityVersion: () => sessions.authorityVersion,
      clearCommunityAccess: sessions.clearCommunityAccess,
      acceptRegistrationSession: sessions.acceptSession,
    );
  }
  final _Harness h;
  final directory = ChannelDirectoryController(repository: _Channels());
  late final AuthSessionController sessions;
  late final AuthFlows flow;
  int entered = 0;
  Future<void> mount(WidgetTester tester) async {
    const qaFont = String.fromEnvironment('AUTH_UI_QA_FONT');
    if (qaFont.isNotEmpty) {
      await tester.runAsync(() async {
        final bytes = await File(qaFont).readAsBytes();
        await (FontLoader(
          'RegistrationQa',
        )..addFont(Future.value(ByteData.sublistView(bytes)))).load();
        const iconFont = String.fromEnvironment('AUTH_UI_QA_ICON_FONT');
        if (iconFont.isNotEmpty) {
          final icons = await File(iconFont).readAsBytes();
          await (FontLoader(
            'MaterialIcons',
          )..addFont(Future.value(ByteData.sublistView(icons)))).load();
        }
      });
    }
    await sessions.start();
    await flow.restorePending();
    await tester.pumpWidget(
      RepaintBoundary(
        key: const ValueKey('registration-qa-surface'),
        child: MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: ThemeData(
            fontFamily: qaFont.isEmpty ? null : 'RegistrationQa',
            useMaterial3: true,
          ),
          home: Builder(
            builder: (context) => Scaffold(
              body: TextButton(
                onPressed: () => Navigator.of(context).push(
                  MaterialPageRoute<void>(
                    builder: (_) => AuthScreen(
                      sessions: sessions,
                      flows: flow,
                      onAuthenticated: () => entered++,
                    ),
                  ),
                ),
                child: const Text('开始使用'),
              ),
            ),
          ),
        ),
      ),
    );
    await open(tester);
  }

  Future<void> open(WidgetTester tester) async {
    await tester.tap(find.text('开始使用'));
    await tester.pumpAndSettle();
  }

  Future<void> close(WidgetTester tester) async {
    await tester.pumpWidget(const SizedBox());
    flow.dispose();
    sessions.dispose();
    directory.dispose();
  }

  Future<void> capture(WidgetTester tester, String name) async {
    const directory = String.fromEnvironment('AUTH_UI_QA_DIR');
    if (directory.isEmpty) return;
    final boundary = tester.renderObject<RenderRepaintBoundary>(
      find.byKey(const ValueKey('registration-qa-surface')),
    );
    await tester.runAsync(() async {
      final rendered = await boundary.toImage();
      final bytes = await rendered.toByteData(format: ui.ImageByteFormat.png);
      await Directory(directory).create(recursive: true);
      await File('$directory/$name.png')
          .writeAsBytes(bytes!.buffer.asUint8List());
      rendered.dispose();
    });
  }
}

void main() {
  for (final timeout in [false, true]) {
    testWidgets(
      'registration timeout=$timeout returns to complete ordinary login',
      (tester) async {
        tester.view.physicalSize = const Size(375, 812);
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.resetPhysicalSize);
        addTearDown(tester.view.resetDevicePixelRatio);
        final h = _Harness();
        await h.seedEligibility();
        if (timeout) {
          h.api.onRegistration = () async =>
              throw const AuthFailure(kind: AuthFailureKind.timeout);
        }
        final screen = _Screen(h);
        await screen.mount(tester);
        await tester.tap(find.text('创建账号'));
        await tester.pumpAndSettle();
        await tester.enterText(
          find.byKey(const ValueKey('registration-username')),
          'private_user',
        );
        await tester.enterText(
          find.byKey(const ValueKey('registration-password')),
          'IndependentPassword123!',
        );
        await tester.tap(find.byKey(const ValueKey('registration-next')));
        await tester.pumpAndSettle();
        expect(
          find.text(timeout ? '注册结果暂未确认，请尝试登录，也可以使用校邮登录或找回账号。' : '注册成功，请重新登陆'),
          findsOneWidget,
        );
        expect(screen.entered, 0);
        expect(screen.sessions.isAuthenticated, isFalse);
        expect(h.store.current!.session, isNull);
        expect(screen.flow.hasPendingRegistration, timeout);
        await screen.capture(
          tester,
          timeout
              ? 'registration-unknown-notice'
              : 'registration-success-notice',
        );
        await tester.tap(find.text('确定'));
        await tester.pumpAndSettle();
        expect(find.byKey(const ValueKey('login-password')), findsOneWidget);
        expect(find.text('切换邮箱登录'), findsOneWidget);
        expect(find.text('找回账号'), findsOneWidget);
        expect(find.text('创建账号'), findsOneWidget);
        await screen.capture(
          tester,
          timeout ? 'registration-unknown-login' : 'registration-success-login',
        );
        if (!timeout) {
          expect(
            h.api.revoked,
            contains(AuthCrypto.revocationSecret(f.session.token)),
          );
        }
        await tester.tap(find.byTooltip('返回登录'));
        await tester.pumpAndSettle();
        await screen.open(tester);
        expect(find.byKey(const ValueKey('login-password')), findsOneWidget);
        expect(find.text('开户结果待核对'), findsNothing);
        await screen.close(tester);
      },
    );
  }

  testWidgets(
    'cold restart shows all login entries and school-email login enters original account',
    (tester) async {
      final original = _Harness();
      await original.seedUnknown();
      final h = _Harness();
      h.vault.value = original.vault.value;
      final screen = _Screen(h);
      await screen.mount(tester);
      expect(find.byKey(const ValueKey('login-password')), findsOneWidget);
      expect(find.text('创建账号'), findsOneWidget);
      await tester.tap(find.text('切换邮箱登录'));
      await tester.pumpAndSettle();
      await tester.enterText(
        find.byKey(const ValueKey('email-login-address')),
        'Exact+Alias',
      );
      await tester.tap(find.byKey(const ValueKey('email-login-request')));
      await tester.pumpAndSettle();
      await tester.enterText(
        find.byKey(const ValueKey('email-login-code')),
        '123456',
      );
      await tester.tap(find.byKey(const ValueKey('login-submit')));
      await tester.pumpAndSettle();
      expect(screen.sessions.isAuthenticated, isTrue);
      expect(screen.entered, 1);
      expect(h.store.current!.registration, isNull);
      await screen.sessions.logout();
      await tester.pumpAndSettle();
      expect(find.text('欢迎回来'), findsOneWidget);
      expect(find.text('找回账号'), findsOneWidget);
      await screen.close(tester);
    },
  );

  for (final result in RegistrationResult.values) {
    testWidgets(
      'create account from old pending registration resolves $result',
      (tester) async {
        final h = _Harness();
        await h.seedUnknown();
        h.api.result = result;
        final screen = _Screen(h);
        await screen.mount(tester);
        await tester.tap(find.text('创建账号'));
        await tester.pumpAndSettle();
        expect(h.api.registrations, isEmpty);
        expect(h.api.queries, hasLength(1));
        if (result == RegistrationResult.notCommitted) {
          expect(
            find.byKey(const ValueKey('registration-email')),
            findsOneWidget,
          );
          expect(find.text('Exact+Alias'), findsOneWidget);
          await tester.tap(find.byTooltip('返回登录'));
          await tester.pumpAndSettle();
        } else {
          expect(
            find.text(
              result == RegistrationResult.committed
                  ? '注册成功，请重新登陆'
                  : '上次注册仍在处理中，请稍后重试；也可以使用校邮登录或找回账号。',
            ),
            findsOneWidget,
          );
          await tester.tap(find.text('确定'));
          await tester.pumpAndSettle();
        }
        expect(find.byKey(const ValueKey('login-password')), findsOneWidget);
        expect(find.text('找回账号'), findsOneWidget);
        await screen.close(tester);
      },
    );
  }

  test('late registration response only revokes its own token and preserves a newer login', () async {
    final h = _Harness();
    await h.seedEligibility();
    final flow = h.make();
    final reply = Completer<AuthSession>();
    h.api.onRegistration = () => reply.future;
    final operation = flow.createRegistration(
      'private_user',
      'IndependentPassword123!',
    );
    await f.until(() => h.api.registrations.isNotEmpty);
    final newer = SessionRecord(
      token: f.bytes(32, 98),
      accountId: f.session.accountId,
      expiresAt: f.session.expiresAt,
    );
    await h.store.update((s) => s.copyWith(session: newer));
    h.authority++;
    flow.dispose();
    reply.complete(
      AuthSession(
        accountId: f.session.accountId,
        sessionToken: f.session.token,
        expiresAt: f.session.expiresAt,
      ),
    );
    await operation;
    expect(h.store.current!.session!.token, newer.token);
    expect(
      h.api.revoked,
      contains(AuthCrypto.revocationSecret(f.session.token)),
    );
    expect(
      h.api.revoked,
      isNot(contains(AuthCrypto.revocationSecret(newer.token))),
    );
  });
  for (final result in RegistrationResult.values) {
    test(
      'registration query $result uses original key without replay',
      () async {
        final h = _Harness();
        await h.seedUnknown();
        final original = h.store.current!.registration!;
        final flow = h.make();
        h.api.result = result;
        await flow.restorePending();
        await flow.reconcileRegistration();
        expect(h.api.queries.single, {
          'id': original['commit']['intentId'],
          'key': original['commit']['key'],
        });
        expect(h.api.registrations, isEmpty);
        expect(h.store.current!.session, isNull);
        if (result == RegistrationResult.notCommitted) {
          expect(flow.hasPendingRegistration, isFalse);
          expect(h.store.current!.registration!['seed'], original['seed']);
          expect(h.store.current!.registration!['slotId'], original['slotId']);
          expect(h.store.current!.registration!['ticket'], isNull);
          await flow.requestOtp('Exact+Alias@hainanu.edu.cn');
          expect(flow.status, AuthFlowStatus.otpRequested);
          expect(h.store.current!.registration!['slotId'], original['slotId']);
        } else if (result == RegistrationResult.pending) {
          expect(flow.hasPendingRegistration, isTrue);
          final resumed = h.make();
          await resumed.restorePending();
          expect(resumed.status, AuthFlowStatus.registrationUnknown);
          resumed.dispose();
        } else {
          expect(flow.hasPendingRegistration, isFalse);
          expect(h.store.current!.registration, isNull);
          expect(flow.status, AuthFlowStatus.registrationComplete);
        }
        flow.dispose();
      },
    );
  }

  test('query unavailable or late authority retains original anchor', () async {
    final h = _Harness();
    await h.seedUnknown();
    final anchor = h.store.current!.registration!['commit'];
    final flow = h.make();
    h.api.resultFailure = const AuthFailure(kind: AuthFailureKind.unavailable);
    await flow.reconcileRegistration();
    expect(h.store.current!.registration!['commit'], anchor);
    h.api.resultFailure = null;
    h.api.response = Completer<RegistrationResult>();
    final operation = flow.reconcileRegistration();
    await f.until(() => h.api.queries.length == 2);
    h.authority++;
    h.api.response!.complete(RegistrationResult.notCommitted);
    await operation;
    expect(h.store.current!.registration!['commit'], anchor);
    flow.dispose();
  });

  testWidgets(
    'unsubmitted email reset after restart does not replace ordinary login',
    (tester) async {
      final original = _Harness();
      final flow = original.make();
      await flow.requestEmailReset('owned@hainanu.edu.cn');
      await flow.completeEmailReset('123456');
      final h = _Harness();
      h.vault.value = original.vault.value;
      final screen = _Screen(h);
      await screen.mount(tester);
      expect(find.byKey(const ValueKey('login-password')), findsOneWidget);
      expect(find.text('切换邮箱登录'), findsOneWidget);
      await tester.tap(find.text('找回账号'));
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('reset-new-password')), findsOneWidget);
      await screen.close(tester);
      flow.dispose();
    },
  );

  for (final sameEmail in [true, false]) {
    test(
      'verified email login resolves only matching registration $sameEmail',
      () async {
        final h = _Harness();
        await h.seedUnknown();
        final flow = h.make();
        await flow.restorePending();
        await flow.requestEmailLogin(
          sameEmail ? 'Exact+Alias@hainanu.edu.cn' : 'other@hainanu.edu.cn',
        );
        await flow.completeEmailLogin('123456');
        expect(flow.error, isNull);
        expect(h.store.current!.session, isNotNull);
        expect(flow.hasPendingRegistration, !sameEmail);
        await h.store.update((s) => s.copyWith(clearSession: true));
        final resumed = h.make();
        await resumed.restorePending();
        expect(resumed.hasPendingRegistration, !sameEmail);
        resumed.dispose();
        flow.dispose();
      },
    );
  }

  testWidgets(
    'unknown registration can switch login methods and reset without username',
    (tester) async {
      final h = _Harness();
      await h.seedUnknown();
      final flow = h.make();
      await flow.restorePending();
      final directory = ChannelDirectoryController(repository: _Channels());
      final sessions = AuthSessionController(
        api: h.api,
        store: h.store,
        directory: directory,
      );
      await sessions.start();
      await tester.pumpWidget(
        MaterialApp(
          home: AuthScreen(sessions: sessions, flows: flow),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('login-password')), findsOneWidget);
      expect(find.text('开户结果待核对'), findsNothing);
      expect(find.text('切换邮箱登录'), findsOneWidget);
      expect(find.text('找回账号'), findsOneWidget);
      await tester.tap(find.text('切换邮箱登录'));
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('email-login-address')), findsOneWidget);
      await tester.tap(find.text('找回账号'));
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('reset-email')), findsOneWidget);
      await tester.enterText(
        find.byKey(const ValueKey('reset-email')),
        'Exact+Alias',
      );
      await tester.tap(find.text('发送重置验证码'));
      await tester.pumpAndSettle();
      expect(flow.error, isNull);
      await tester.enterText(find.byKey(const ValueKey('reset-otp')), '123456');
      await tester.ensureVisible(find.text('验证校邮'));
      await tester.tap(find.text('验证校邮'));
      await tester.pumpAndSettle();
      expect(flow.hasPendingRegistration, isFalse);
      await tester.tap(find.byTooltip('返回登录'));
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('login-password')), findsOneWidget);
      expect(find.text('切换邮箱登录'), findsOneWidget);
      await tester.tap(find.text('找回账号'));
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('reset-new-password')), findsOneWidget);
      await tester.enterText(
        find.byKey(const ValueKey('reset-new-password')),
        'NewIndependent123!',
      );
      await tester.tap(find.text('重置密码'));
      await tester.pumpAndSettle();
      expect(find.text('密码已重设'), findsOneWidget);
      await tester.tap(find.text('前往登录'));
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('login-password')), findsOneWidget);
      expect(h.store.current!.session, isNull);
      expect(h.api.sessions, isEmpty);
      expect(h.api.resetCommits, hasLength(1));
      await tester.pumpWidget(const SizedBox());
      flow.dispose();
      sessions.dispose();
      directory.dispose();
    },
  );
}
