import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';

import 'auth_flows_test.dart' as f;
import 'email_login_test.dart' as login;

class ResetApi extends login.EmailApi implements EmailResetApi {
  final intents = <Map<String, String>>[];
  Future<PasswordResetIntent> Function()? onIntent;
  @override
  Future<OtpRequestAccepted> requestEmailResetOtp({
    required String email,
    required String installationId,
    required String idempotencyKey,
  }) => requestEmailLoginOtp(
    email: email,
    installationId: installationId,
    idempotencyKey: idempotencyKey,
  );
  @override
  Future<String?> confirmEmailResetOtp({
    required String flowId,
    required String otp,
    required String binding,
    required String installationId,
    required String idempotencyKey,
  }) => confirmEmailLoginOtp(
    flowId: flowId,
    otp: otp,
    binding: binding,
    installationId: installationId,
    idempotencyKey: idempotencyKey,
  );
  @override
  Future<PasswordResetIntent> createEmailResetIntent({
    required String proof,
    required String installationId,
    required String idempotencyKey,
  }) async {
    intents.add({'proof': proof, 'id': installationId, 'key': idempotencyKey});
    return onIntent != null
        ? await onIntent!()
        : PasswordResetIntent(
            resetIntentId: f.bytes(32, 77),
            expiresAt: f.now.add(const Duration(minutes: 10)),
            username: 'private_user',
            newRecoveryCode: '',
          );
  }

  @override
  Future<RegistrationIntent> createRegistrationIntent({
    required String registrationTicket,
    required String username,
    required String password,
    required String installationId,
  }) async => RegistrationIntent(
    intentId: f.bytes(32, 21),
    challenge: f.bytes(32, 22),
    expiresAt: f.now.add(const Duration(minutes: 10)),
    recoveryCode: '',
  );
}

class ResetHarness {
  final vault = f.Vault(), api = ResetApi();
  late final store = AuthStore(vault);
  int adopted = 0, authority = 0;
  AuthFlows make() => AuthFlows(
    api: api,
    store: store,
    legacyRecoveryEnabled: false,
    clock: () => f.now,
    sessionAuthorityVersion: () => authority,
    clearCommunityAccess: () {},
    acceptRegistrationSession: (_) async {
      adopted++;
    },
  );
}

class _Channels implements ChannelRepository {
  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

void main() {
  for (final code in ['RESET_INTENT_INVALID', 'RESET_INTENT_EXPIRED']) {
    for (final terminal in [false, true]) {
      test(
        '$code with terminal=$terminal only restarts after authoritative non-commit',
        () async {
          final h = ResetHarness();
          final flow = h.make();
          await flow.requestEmailReset('owned@hainanu.edu.cn');
          await flow.completeEmailReset('123456');
          final original = h.store.current!.emailReset!['intentId'];
          h.api.onCommitReset = () async =>
              throw AuthFailure(kind: AuthFailureKind.rejected, code: code);
          h.api.resetResult = terminal
              ? PasswordResetResult.notCommitted
              : PasswordResetResult.pending;
          await flow.commitReset('NewPassword123!', '');
          expect(flow.canCorrectResetPassword, isFalse);
          expect(
            h.store.current!.pendingReset!['state'],
            terminal ? 'NOT_COMMITTED' : 'UNKNOWN',
          );
          if (terminal) {
            expect(h.store.current!.emailReset, isNull);
            final resumed = h.make();
            await resumed.restorePending();
            expect(resumed.canCorrectResetPassword, isFalse);
            await resumed.acknowledgeResetResult();
            await resumed.requestEmailReset('owned@hainanu.edu.cn');
            expect(h.store.current!.emailReset!['intentId'], isNull);
            expect(h.store.current!.pendingReset, isNull);
          } else {
            expect(h.store.current!.emailReset!['intentId'], original);
            await flow.acknowledgeResetResult();
            final retained = await h.store.read();
            expect(retained.pendingReset!['state'], 'UNKNOWN');
            expect(retained.emailReset!['intentId'], original);
          }
        },
      );
    }
  }

  test('restarting a terminal reset discards its stale email intent', () async {
    final h = ResetHarness();
    final flow = h.make();
    await flow.requestEmailReset('owned@hainanu.edu.cn');
    await flow.completeEmailReset('123456');
    h.api.onCommitReset = () async =>
        throw const AuthFailure(kind: AuthFailureKind.unavailable);
    await flow.commitReset('NewPassword123!', '');
    h.api.resetResult = PasswordResetResult.notCommitted;
    await flow.reconcileReset();
    await flow.acknowledgeResetResult();
    expect(h.store.current!.emailReset, isNull);
    await flow.requestEmailReset('owned@hainanu.edu.cn');
    expect(h.store.current!.emailReset!['intentId'], isNull);
  });
  test('policy refusal requires terminal result and survives restart before correction', () async {
    final h = ResetHarness();
    final flow = h.make();
    await flow.requestEmailReset('owned@hainanu.edu.cn');
    await flow.completeEmailReset('123456');
    h.api.onCommitReset = () async => throw const AuthFailure(
      kind: AuthFailureKind.rejected,
      code: 'PASSWORD_POLICY_FAILED',
    );
    h.api.resetResult = PasswordResetResult.notCommitted;
    await flow.commitReset('InvalidPassword123!', '');
    expect(flow.status, AuthFlowStatus.resetNotCommitted);
    expect(h.store.current!.pendingReset!['state'], 'NOT_COMMITTED');
    final oldKey = h.api.resetCommits.single['key'];
    final resumed = h.make();
    await resumed.restorePending();
    expect(resumed.canCorrectResetPassword, isTrue);
    await resumed.correctResetPassword();
    expect(resumed.status, AuthFlowStatus.resetCodeConfirmation);
    expect(resumed.username, 'private_user');
    h.api.onCommitReset = null;
    await resumed.commitReset('CorrectedPassword123!', '');
    expect(h.api.resetCommits.last['key'], isNot(oldKey));
    expect(resumed.status, AuthFlowStatus.resetCommitted);
    expect(h.store.current!.emailReset, isNull);
  });

  test(
    'unavailable policy receipt remains unknown and cannot be corrected',
    () async {
      final h = ResetHarness();
      final f = h.make();
      await f.requestEmailReset('owned@hainanu.edu.cn');
      await f.completeEmailReset('123456');
      h.api.onCommitReset = () async => throw const AuthFailure(
        kind: AuthFailureKind.rejected,
        code: 'PASSWORD_POLICY_FAILED',
      );
      h.api.resetResultFailure = const AuthFailure(
        kind: AuthFailureKind.unavailable,
      );
      await f.commitReset('NewPassword123!', '');
      expect(h.store.current!.pendingReset!['state'], 'UNKNOWN');
      expect(f.canCorrectResetPassword, isFalse);
    },
  );
  test(
    'email reset survives restart and commits without a code or login',
    () async {
      final h = ResetHarness(), flow = ResetHarness();
      // An independent email-login operation must never receive reset proof state.
      await h.make().requestEmailReset('owned@hainanu.edu.cn');
      await h.make().completeEmailReset('123456');
      expect(h.store.current!.emailLogin, isNull);
      expect(h.store.current!.emailReset!['intentId'], isNotNull);
      expect(h.api.sessions, isEmpty);
      final resumed = h.make();
      await resumed.restorePending();
      expect(resumed.status, AuthFlowStatus.resetCodeConfirmation);
      await resumed.commitReset('NewIndependentPassword123!', '');
      expect(resumed.status, AuthFlowStatus.resetCommitted);
      expect(h.adopted, 0);
      expect(h.store.current!.emailReset, isNull);
      expect(h.store.current!.session, isNull);
      expect(h.store.current!.pendingReset!['state'], 'COMMITTED');
      expect(h.vault.value, isNot(contains('NewIndependentPassword123!')));
      expect(h.vault.value, isNot(contains('123456')));
      await resumed.acknowledgeResetResult();
      expect(h.store.current!.pendingReset, isNull);
      await flow.make().beginCodeReset('old code');
      expect(flow.api.proofCalls, 0);
    },
  );

  test(
    'unknown send and confirmation retain the exact original operation',
    () async {
      final h = ResetHarness();
      h.api.sendTimeout = true;
      await h.make().requestEmailReset('owned@hainanu.edu.cn');
      final send = h.api.sends.single;
      h.api.sendTimeout = false;
      await h.make().requestEmailReset('owned@hainanu.edu.cn');
      expect(h.api.sends.last, send);
      h.api.confirmTimeout = true;
      await h.make().completeEmailReset('123456');
      final confirm = h.api.confirms.single;
      h.api.confirmTimeout = false;
      await h.make().completeEmailReset('654321');
      expect(h.api.confirms.last, confirm);
      expect(h.adopted, 0);
    },
  );

  test(
    'late proof reply after a new login cannot prepare or commit reset',
    () async {
      final h = ResetHarness(),
          flowReady = Completer<void>(),
          reply = Completer<PasswordResetIntent>();
      final flow = h.make();
      await flow.requestEmailReset('owned@hainanu.edu.cn');
      h.api.onIntent = () {
        flowReady.complete();
        return reply.future;
      };
      final pending = flow.completeEmailReset('123456');
      await flowReady.future;
      h.authority++;
      reply.complete(
        PasswordResetIntent(
          resetIntentId: f.bytes(32, 77),
          expiresAt: f.now.add(const Duration(minutes: 10)),
          username: 'private_user',
          newRecoveryCode: '',
        ),
      );
      await pending;
      await flow.commitReset('NewIndependentPassword123!', '');
      expect(h.api.resetCommits, isEmpty);
      expect(h.adopted, 0);
    },
  );

  test('unknown password reset is queried rather than sent twice', () async {
    final h = ResetHarness();
    final flow = h.make();
    await flow.requestEmailReset('owned@hainanu.edu.cn');
    await flow.completeEmailReset('123456');
    h.api.onCommitReset = () async {
      throw const AuthFailure(kind: AuthFailureKind.timeout);
    };
    await flow.commitReset('NewIndependentPassword123!', '');
    expect(flow.status, AuthFlowStatus.resetUnknown);
    h.api.resetResult = PasswordResetResult.committed;
    final restarted = h.make();
    await restarted.restorePending();
    await restarted.reconcileReset();
    expect(restarted.status, AuthFlowStatus.resetCommitted);
    expect(h.api.resetCommits.length, 1);
    expect(h.adopted, 0);
  });

  test('unregistered mailbox cannot create reset intent', () async {
    final h = ResetHarness();
    h.api.unregistered = true;
    final flow = h.make();
    await flow.requestEmailReset('new@hainanu.edu.cn');
    await flow.completeEmailReset('123456');
    expect(h.api.intents, isEmpty);
    expect(h.api.registrations, isEmpty);
    expect(h.store.current!.emailReset!['unregistered'], true);
  });

  for (final source in ['verifier', 'community', 'unavailable']) {
    testWidgets('email recovery distinguishes missing account from $source', (
      tester,
    ) async {
      const missingAccount = '该邮箱未找到账户信息，请先注册';
      final h = ResetHarness();
      h.api.unregistered = source == 'verifier';
      if (source != 'verifier') {
        h.api.onIntent = () async => throw AuthFailure(
          kind: AuthFailureKind.rejected,
          code: source == 'community'
              ? 'EMAIL_UNREGISTERED'
              : 'SERVICE_UNAVAILABLE',
        );
      }
      final directory = ChannelDirectoryController(repository: _Channels());
      final sessions = AuthSessionController(
        api: h.api,
        store: h.store,
        directory: directory,
      );
      await sessions.start();
      final flow = h.make();
      await tester.pumpWidget(
        MaterialApp(
          home: AuthScreen(sessions: sessions, flows: flow),
        ),
      );
      await tester.pumpAndSettle();
      await tester.tap(find.text('找回账号'));
      await tester.pumpAndSettle();
      await tester.enterText(
        find.byKey(const ValueKey('reset-email')),
        'owned',
      );
      await tester.tap(find.text('发送重置验证码'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byKey(const ValueKey('reset-otp')), '123456');
      await tester.ensureVisible(find.text('验证校邮'));
      await tester.tap(find.text('验证校邮'));
      await tester.pumpAndSettle();

      expect(h.api.confirms, hasLength(1));
      expect(
        find.text(missingAccount),
        source == 'unavailable' ? findsNothing : findsOneWidget,
      );
      expect(find.byKey(const ValueKey('reset-new-password')), findsNothing);
      expect(h.api.registrations, isEmpty);
      expect(h.api.resetCommits, isEmpty);
      expect(h.adopted, 0);
      if (source == 'community') {
        expect(flow.error?.code, 'EMAIL_UNREGISTERED');
        expect(h.store.current!.emailReset, isNull);
        expect(find.text('操作未完成，请检查输入或核对原操作。'), findsNothing);
        await tester.tap(find.text('确定'));
        await tester.pumpAndSettle();
        expect(find.byKey(const ValueKey('reset-email')), findsOneWidget);
        expect(find.text('发送重置验证码'), findsOneWidget);
      } else if (source == 'verifier') {
        expect(h.api.intents, isEmpty);
        expect(h.store.current!.emailReset!['unregistered'], true);
      } else {
        expect(flow.error?.code, 'SERVICE_UNAVAILABLE');
        expect(h.store.current!.emailReset!['proof'], isNotNull);
      }
      await tester.pumpWidget(const SizedBox());
      flow.dispose();
      sessions.dispose();
      directory.dispose();
    });
  }

  test(
    'registration commits immediately when the backend omits recovery code',
    () async {
      final original = f.Harness();
      await original.eligibility();
      final api = ResetApi();
      int adopted = 0;
      final flow = AuthFlows(
        api: api,
        store: original.store,
        legacyRecoveryEnabled: false,
        clock: () => f.now,
        clearCommunityAccess: () {},
        acceptRegistrationSession: (_) async {
          adopted++;
        },
      );
      await flow.createRegistration(
        'private_user',
        'NewIndependentPassword123!',
      );
      expect(flow.status, AuthFlowStatus.registrationComplete);
      expect(api.registrations.length, 1);
      expect(adopted, 1);
      expect(flow.recoveryCode, isNull);
    },
  );

  for (final code in ['PASSWORD_POLICY_FAILED', 'RESET_INTENT_INVALID']) {
    testWidgets('$code terminal page can restart email verification', (
      tester,
    ) async {
      final h = ResetHarness();
      final directory = ChannelDirectoryController(repository: _Channels());
      final sessions = AuthSessionController(
        api: h.api,
        store: h.store,
        directory: directory,
      );
      await sessions.start();
      final flow = h.make();
      await flow.requestEmailReset('owned@hainanu.edu.cn');
      await flow.completeEmailReset('123456');
      h.api.onCommitReset = () async =>
          throw AuthFailure(kind: AuthFailureKind.rejected, code: code);
      h.api.resetResult = PasswordResetResult.notCommitted;
      await flow.commitReset('NewPassword123!', '');
      await tester.pumpWidget(
        MaterialApp(
          home: AuthScreen(sessions: sessions, flows: flow),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('本次重设未提交'), findsOneWidget);
      expect(
        find.text('重新输入密码'),
        code == 'PASSWORD_POLICY_FAILED' ? findsOneWidget : findsNothing,
      );
      await tester.ensureVisible(find.text('重新开始恢复'));
      await tester.tap(find.text('重新开始恢复'));
      await tester.pumpAndSettle();
      expect(find.byKey(const ValueKey('reset-email')), findsOneWidget);
      expect(h.store.current!.pendingReset, isNull);
      expect(h.store.current!.emailReset, isNull);
      expect(h.api.resetCommits, hasLength(1));
      await tester.pumpWidget(const SizedBox());
      flow.dispose();
      sessions.dispose();
      directory.dispose();
    });
  }

  testWidgets('forgot password shows email only and completes back at login', (
    tester,
  ) async {
    final h = ResetHarness(),
        directory = ChannelDirectoryController(repository: _Channels());
    final sessions = AuthSessionController(
      api: h.api,
      store: h.store,
      directory: directory,
    );
    await sessions.start();
    final flow = AuthFlows(
      api: h.api,
      store: h.store,
      legacyRecoveryEnabled: false,
      clock: () => f.now,
      clearCommunityAccess: sessions.clearCommunityAccess,
      acceptRegistrationSession: sessions.acceptSession,
    );
    await tester.pumpWidget(
      MaterialApp(
        home: AuthScreen(sessions: sessions, flows: flow),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('找回账号'));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('recovery-code')), findsNothing);
    expect(find.textContaining('Passkey'), findsNothing);
    await tester.enterText(find.byKey(const ValueKey('reset-email')), 'owned');
    await tester.tap(find.text('发送重置验证码'));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const ValueKey('reset-otp')), '123456');
    await tester.ensureVisible(find.text('验证校邮'));
    await tester.tap(find.text('验证校邮'));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('recovery-confirmation')), findsNothing);
    await tester.enterText(
      find.byKey(const ValueKey('reset-new-password')),
      'NewIndependentPassword123!',
    );
    await tester.tap(find.text('重置密码'));
    await tester.pumpAndSettle();
    expect(find.text('密码已重设'), findsOneWidget);
    await tester.tap(find.text('前往登录'));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('login-password')), findsOneWidget);
    expect(h.api.sessions, isEmpty);
    await tester.pumpWidget(const SizedBox());
    flow.dispose();
    sessions.dispose();
    directory.dispose();
  });
}
