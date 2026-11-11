import 'dart:convert';
import 'dart:io';

import 'identity_https_scenarios.dart';
import 'verifier_gate_https_scenarios.dart';

import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_auth_passkey/hnuhole_auth_passkey.dart';
import 'package:hnuhole_mobile/src/auth/auth_crypto.dart';
import 'package:hnuhole_mobile/src/auth/auth_flows.dart';
import 'package:hnuhole_mobile/src/auth/auth_models.dart';
import 'package:hnuhole_mobile/src/auth/auth_session_controller.dart';
import 'package:hnuhole_mobile/src/auth/auth_store.dart';
import 'package:hnuhole_mobile/src/auth/security_management_controller.dart';
import 'package:hnuhole_mobile/src/auth/http_auth_api.dart';
import 'package:hnuhole_mobile/src/identity/identity_api.dart';
import 'package:hnuhole_mobile/src/channels/http_channel_repository.dart';
import 'package:hnuhole_mobile/src/navigation/channel_directory_controller.dart';

/// Only the native persistence port is replaced. Every authentication request
/// traverses the shipped Dart transport, real Go TLS handler, and two real DBs.
/// Directory requests use the shipped transport and real C Gate transaction.
/// This does not certify Keychain/Keystore durability.
class _LabVault implements AuthVault {
  String? value;
  @override
  Future<String?> read() async => value;
  @override
  Future<void> write(String next) async => value = next;
}

void main() {
  final registrationOnly =
      Platform.environment['AUTHLAB_REGISTRATION_FLOW_ONLY'] == '1';
  final configPath = Platform.environment['AUTHLAB_MOBILE_CONFIG'];
  if (configPath == null) {
    test(
      'real public C/V HTTPS/PostgreSQL runner required',
      () {},
      skip: 'Run services/api/authlab/run-mobile-isolated.sh',
    );
    return;
  }
  test(
    registrationOnly
        ? 'real C/V TLS registration, both logins, email reset and cold restart'
        : 'real C/V TLS signup, takeover, revoke, reset, closure and Gate faults',
    () async {
      final config = jsonDecode(
        File(configPath).readAsStringSync(),
      ) as Map<String, dynamic>;
      final trust = SecurityContext(withTrustedRoots: false)
        ..setTrustedCertificates(config['caPath'] as String);
      final api = HttpAuthApi(
        communityBaseUri: Uri.parse(config['communityOrigin'] as String),
        verifierBaseUri: Uri.parse(config['verifierOrigin'] as String),
        client: HttpClient(context: trust),
        timeout: const Duration(seconds: 20),
      );
      final control = HttpClient(context: trust);
      final repository = HttpChannelRepository(
        baseUri: Uri.parse(config['communityOrigin'] as String),
        client: HttpClient(context: trust),
        timeout: const Duration(seconds: 20),
      );
      addTearDown(() {
        api.close();
        control.close(force: true);
        repository.close();
      });
      Future<Map<String, dynamic>> fixture(
        String action, [
        Map<String, String> body = const {},
      ]) async {
        final request = await control.postUrl(
          Uri.parse(config['controlOrigin'] as String).resolve('/$action'),
        );
        request.followRedirects = false;
        request.headers.set(
          'Authorization',
          'LabControl ${config['controlToken']}',
        );
        request.headers.contentType = ContentType.json;
        request.write(jsonEncode(body));
        final response = await request.close();
        expect(
          response.statusCode,
          200,
          reason: 'Private fixture $action failed',
        );
        return jsonDecode(await utf8.decoder.bind(response).join())
            as Map<String, dynamic>;
      }

      final vault = _LabVault();
      var store = AuthStore(vault, scope: 'isolated-real-CV-test');
      var directory = ChannelDirectoryController(repository: repository);
      var sessions = AuthSessionController(
        api: api,
        store: store,
        directory: directory,
      );
      var clientClock = DateTime.now().toUtc();
      AuthFlows flowsForCurrentStore() => AuthFlows(
        api: api,
        store: store,
        acceptRegistrationSession: sessions.acceptSession,
        clearCommunityAccess: sessions.clearCommunityAccess,
        clock: () => clientClock,
        legacyRecoveryEnabled: !registrationOnly,
        sessionAuthorityVersion: () => sessions.authorityVersion,
      );
      var flows = flowsForCurrentStore();
      addTearDown(() {
        flows.dispose();
        sessions.dispose();
        directory.dispose();
        store.dispose();
      });
      Future<void> restartClient() async {
        flows.dispose();
        sessions.dispose();
        directory.dispose();
        store.dispose();
        store = AuthStore(vault, scope: 'isolated-real-CV-test');
        directory = ChannelDirectoryController(repository: repository);
        sessions = AuthSessionController(
          api: api,
          store: store,
          directory: directory,
        );
        flows = flowsForCurrentStore();
        await sessions.start();
        await flows.restorePending();
      }

      Future<void> requestAfterCooldown(Future<void> Function() request) async {
        await request();
        if (flows.error?.statusCode == 429) {
          final wait = flows.error!.retryAfterSeconds ?? 60;
          expect(wait, inInclusiveRange(0, 60));
          expect(store.current!.registration, isNotNull);
          await Future<void>.delayed(Duration(seconds: wait + 1));
          clientClock = DateTime.now().toUtc();
          await request();
        }
        expect(
          flows.error,
          isNull,
          reason: 'email request: ${flows.error?.code}',
        );
      }

      const email = 'synthetic-mobile@hainanu.edu.cn';
      const username = 'synthetic_mobile';
      const password = 'Isolated-Mobile-Long-Password-2026!';
      const resetPassword = 'Isolated-Mobile-Changed-Password-2026!';
      await sessions.start();
      expect(sessions.status, AuthStatus.signedOut);
      await fixture('drop-next', {'path': '/api/v1/eligibility/otp-requests'});
      await flows.requestOtp(email);
      expect(flows.status, AuthFlowStatus.otpRequestUnknown);
      final originalOtpKey = store.current!.registration!['request']['key'];
      clientClock = clientClock.add(const Duration(seconds: 41));
      await restartClient();
      expect(flows.status, AuthFlowStatus.otpRequestUnknown);
      await flows.reconcileOtpRequest();
      expect(flows.status, AuthFlowStatus.otpRequested);
      expect(store.current!.registration!['request']['key'], originalOtpKey);
      final mail = await fixture('otp', {'email': email});
      expect(mail['mailCalls'], 1);
      await flows.confirmOtp(mail['code'] as String);
      expect(flows.status, AuthFlowStatus.eligibilityReady);
      final slot = store.current!.registration!['slotId'] as String;
      Map<String, dynamic>? signupAnchor;
      Map<String, dynamic>? unknownRegistration;
      void captureSignupAnchor() {
        final registration = store.current?.registration;
        final commit = registration?['commit'];
        if (commit is Map) {
          signupAnchor = Map<String, dynamic>.from(commit);
          unknownRegistration = Map<String, dynamic>.from(registration!);
        }
      }

      store.addListener(captureSignupAnchor);
      if (registrationOnly) {
        clientClock = DateTime.now().toUtc();
        final originalSeed = store.current!.registration!['seed'];
        await fixture('hold-next-signup');
        await flows.createRegistration(username, password);
        expect(flows.status, AuthFlowStatus.registrationUnknown);
        final originalCommit = store.current!.registration!['commit'];
        await fixture('abandon-held-signup', {
          'intentId': originalCommit['intentId'] as String,
        });
        await restartClient();
        expect(flows.status, AuthFlowStatus.registrationUnknown);
        await flows.reconcileRegistration();
        expect(flows.error, isNull);
        expect(flows.status, AuthFlowStatus.idle);
        expect(store.current!.registration!['slotId'], slot);
        expect(store.current!.registration!['seed'], originalSeed);
        await requestAfterCooldown(() => flows.requestOtp(email));
        final retryMail = await fixture('otp', {'email': email});
        await flows.confirmOtp(retryMail['code'] as String);
        expect(
          flows.error,
          isNull,
          reason: 'renewed proof: ${flows.error?.code}',
        );
        expect(flows.status, AuthFlowStatus.eligibilityReady);
        expect(store.current!.registration!['slotId'], slot);
        // restartClient installed a new store, so move the observer as well.
        store.addListener(captureSignupAnchor);
      }
      await flows.createRegistration(username, password);
      expect(flows.error, isNull, reason: 'registration: ${flows.error?.code}');
      var originalCode = '';
      if (!registrationOnly) {
        expect(flows.status, AuthFlowStatus.registrationCodeShown);
        originalCode = flows.recoveryCode!;
        flows.hideRecoveryCode();
        expect(flows.recoveryCode, isNull);
        await flows.commitRegistration(originalCode);
      }
      var currentRecoveryCode = originalCode;
      store.removeListener(captureSignupAnchor);
      expect(flows.status, AuthFlowStatus.registrationComplete);
      expect(sessions.isAuthenticated, isFalse);
      expect(store.current!.session, isNull);
      await sessions.login(username, password);
      expect(sessions.isAuthenticated, isTrue);
      expect(directory.status, ChannelDirectoryStatus.ready);
      expect(directory.channels, hasLength(7));
      final durableSession = AuthState.parse(
        jsonDecode(vault.value!) as Map<String, dynamic>,
        store.scope,
      ).session!;
      expect(durableSession.expiresAt, sessions.currentSession!.expiresAt);
      expect(durableSession.token, store.current!.session!.token);
      expect(store.current!.registration, isNull);
      if (registrationOnly) {
        expect(signupAnchor, isNotNull);
        final token = store.current!.session!.token;
        final result = await api.registrationResult(
          intentId: signupAnchor!['intentId'] as String,
          idempotencyKey: signupAnchor!['key'] as String,
        );
        expect(result, RegistrationResult.committed);
        expect(store.current!.session!.token, token);
        await restartClient();
        expect(sessions.isAuthenticated, isTrue);
        expect(store.current!.session!.token, token);
        await sessions.logout();
        // Restore the exact durable pre-response anchor to model a process
        // ending after the real server commit but before the client saw it.
        await store.update(
          (s) => s.copyWith(registration: unknownRegistration),
        );
        await restartClient();
        expect(flows.status, AuthFlowStatus.registrationUnknown);
        expect(sessions.isAuthenticated, isFalse);
        await requestAfterCooldown(() => flows.requestEmailLogin(email));
        final loginMail = await fixture('otp', {'email': email});
        await flows.completeEmailLogin(loginMail['code'] as String);
        expect(
          flows.error,
          isNull,
          reason: 'email login: ${flows.error?.code}',
        );
        expect(sessions.isAuthenticated, isTrue);
        expect(store.current!.registration, isNull);
        await restartClient();
        expect(sessions.isAuthenticated, isTrue);
        await sessions.logout();
        await store.update(
          (s) => s.copyWith(registration: unknownRegistration),
        );
        await restartClient();
        expect(flows.status, AuthFlowStatus.registrationUnknown);
        // No username or old password is supplied to email recovery.
        await requestAfterCooldown(() => flows.requestEmailReset(email));
        final resetMail = await fixture('otp', {'email': email});
        await flows.completeEmailReset(resetMail['code'] as String);
        expect(
          flows.error,
          isNull,
          reason: 'reset proof: ${flows.error?.code}',
        );
        expect(flows.status, AuthFlowStatus.resetCodeConfirmation);
        expect(store.current!.registration, isNull);
        expect(sessions.isAuthenticated, isFalse);
        await restartClient();
        expect(flows.status, AuthFlowStatus.resetCodeConfirmation);
        await flows.commitReset(resetPassword, '');
        expect(
          flows.error,
          isNull,
          reason: 'reset commit: ${flows.error?.code}',
        );
        expect(flows.status, AuthFlowStatus.resetCommitted);
        expect(sessions.isAuthenticated, isFalse);
        await restartClient();
        expect(flows.status, AuthFlowStatus.resetCommitted);
        await flows.acknowledgeResetResult();
        expect(flows.status, AuthFlowStatus.idle);
        await sessions.login(username, password);
        expect(sessions.isAuthenticated, isFalse);
        await sessions.login(username, resetPassword);
        expect(sessions.isAuthenticated, isTrue);
        await restartClient();
        expect(sessions.isAuthenticated, isTrue);
        await sessions.logout();
        await restartClient();
        expect(sessions.isAuthenticated, isFalse);
        expect(store.current!.pendingReset, isNull);
        expect(store.current!.emailReset, isNull);
        expect(store.current!.registration, isNull);
        expect(store.current!.session, isNull);
        return;
      }
      final signupToken = store.current!.session!.token;
      final nearExpiry = await fixture('near-expiry', {'token': signupToken});
      final priorDeadline = DateTime.parse(nearExpiry['expiresAt'] as String);
      await directory.load();
      expect(directory.status, ChannelDirectoryStatus.ready);
      expect(directory.channels, hasLength(7));
      final renewedDeadline = store.current!.session!.expiresAt;
      expect(
        renewedDeadline.isAfter(priorDeadline.add(const Duration(days: 23))),
        isTrue,
      );
      expect(
        (await api.currentSession(signupToken)).expiresAt,
        renewedDeadline,
      );
      final persistedRenewal = AuthState.parse(
        jsonDecode(vault.value!) as Map<String, dynamic>,
        store.scope,
      ).session!;
      expect(persistedRenewal.token, signupToken);
      expect(persistedRenewal.expiresAt, renewedDeadline);
      await restartClient();
      expect(store.current!.session!.token, signupToken);
      expect(store.current!.session!.expiresAt, renewedDeadline);
      expect(directory.status, ChannelDirectoryStatus.ready);
      expect(directory.channels, hasLength(7));
      final expiry = await api.renewSession(signupToken);
      expect(expiry.isBefore(store.current!.session!.expiresAt), isFalse);

      await verifyVerifierGateHttpsScenarios(
        api: api,
        communityToken: store.current!.session!.token,
        fixture: (action, body) => fixture(action, body),
      );

      final identityReceiptKey = await verifyIdentityHttpsScenarios(
        api: api,
        store: () => store,
        sessions: () => sessions,
        restartClient: restartClient,
        fixture: (action, body) => fixture(action, body),
        username: username,
        password: password,
      );

      // Real transport/SQL plus one intentionally lost final response. Native
      // Passkey is not invoked in this headless scenario and is tested apart.
      SecurityManagementController managerForCurrentStore() =>
          SecurityManagementController(
            api: api,
            store: store,
            sessions: sessions,
            passkey: NativePasskeyClient(),
          );
      var manager = managerForCurrentStore();
      addTearDown(() => manager.dispose());
      await manager.load();
      expect(manager.status, SecurityManagementStatus.ready);
      expect(
        manager.devices!.currentSignedInAt.isAfter(DateTime.utc(2026)),
        isTrue,
      );
      expect(manager.credentials!.passkeys, isEmpty);
      await manager.beginRecoveryCodeRotation(password);
      expect(manager.status, SecurityManagementStatus.rotationCodeShown);
      currentRecoveryCode = manager.recoveryCode!;
      manager.hideRecoveryCode();
      expect(manager.recoveryCode, isNull);
      expect(vault.value, isNot(contains(currentRecoveryCode)));
      await fixture('drop-next', {
        'path': '/api/v1/auth/recovery-code-rotations/*/confirmations',
      });
      await manager.confirmRecoveryCodeRotation(currentRecoveryCode);
      expect(manager.status, SecurityManagementStatus.pending);
      final rotationKey = store.current!.pendingCredentialChange!['key'];
      expect(vault.value, isNot(contains(currentRecoveryCode)));
      manager.dispose();
      await restartClient();
      manager = managerForCurrentStore();
      await manager.load();
      expect(manager.status, SecurityManagementStatus.committed);
      expect(store.current!.pendingCredentialChange!['key'], rotationKey);
      await manager.acknowledgeResult();
      expect(manager.status, SecurityManagementStatus.ready);
      expect(store.current!.pendingCredentialChange, isNull);

      final secondSession = await api.login(
        username: username,
        password: password,
        installationId: AuthCrypto.randomEncoded(16),
        idempotencyKey: AuthCrypto.randomEncoded(32),
      );
      await expectLater(
        api.currentSession(signupToken),
        throwsA(
          isA<AuthFailure>().having((e) => e.code, 'code', 'session_replaced'),
        ),
      );
      await sessions.retry();
      expect(sessions.status, AuthStatus.signedOut);
      expect(store.current!.session, isNull);
      expect(manager.devices, isNull);
      expect(manager.credentials, isNull);
      await sessions.acceptSession(secondSession);
      expect(sessions.isAuthenticated, isTrue);

      await fixture('drop-next', {'path': '/api/v1/auth/session-revocations'});
      await sessions.logout();
      expect(store.current!.session, isNull);
      expect(store.current!.logouts, hasLength(1));
      expect(store.current!.logouts.single.revocationSecret, isNotNull);
      await restartClient();
      await sessions.retryPendingLogout();
      expect(store.current!.logouts, isEmpty);
      expect(sessions.status, AuthStatus.signedOut);
      await expectLater(
        api.currentSession(secondSession.sessionToken),
        throwsA(isA<AuthFailure>().having((e) => e.unauthorized, '401', true)),
      );

      await sessions.login(username, password);
      expect(sessions.isAuthenticated, isTrue);
      final beforeFreezeToken = store.current!.session!.token;
      await fixture('freeze');
      manager.dispose();
      manager = managerForCurrentStore();
      await manager.load();
      expect(manager.status, SecurityManagementStatus.unavailable);
      expect(manager.devices, isNull);
      expect(manager.credentials, isNull);
      await sessions.retry();
      expect(sessions.status, AuthStatus.unavailable);
      expect(store.current!.session!.token == beforeFreezeToken, isTrue);
      expect(directory.channels, isEmpty);
      await fixture('recover');
      await sessions.retry();
      expect(sessions.status, AuthStatus.signedOut);
      expect(store.current!.session, isNull);
      await sessions.login(username, password);
      expect(sessions.isAuthenticated, isTrue);

      final activeBeforeResetToken = store.current!.session!.token;
      expect(
        (await api.currentSession(activeBeforeResetToken)).accountId,
        store.current!.session!.accountId,
      );
      await flows.beginCodeReset(currentRecoveryCode);
      expect(flows.status, AuthFlowStatus.resetCodeShown);
      final newCode = flows.recoveryCode!;
      flows.hideRecoveryCode();
      // A recovery intent must not revoke this still-live session. The commit,
      // rather than a prior takeover/logout, must cause its later rejection.
      expect(
        (await api.currentSession(activeBeforeResetToken)).accountId,
        store.current!.session!.accountId,
      );
      await fixture('drop-next', {'path': '/api/v1/auth/password-resets'});
      await flows.commitReset(resetPassword, newCode);
      expect(flows.status, AuthFlowStatus.resetUnknown);
      await expectLater(
        api.currentSession(activeBeforeResetToken),
        throwsA(
          isA<AuthFailure>().having(
            (e) => e.unauthorized,
            'active pre-reset session revoked by real C commit',
            true,
          ),
        ),
      );
      final resetKey = store.current!.pendingReset!['key'];
      await restartClient();
      expect(sessions.isAuthenticated, isFalse);
      expect(flows.status, AuthFlowStatus.resetUnknown);
      expect(store.current!.pendingReset!['key'], resetKey);
      await flows.reconcileReset();
      expect(flows.status, AuthFlowStatus.resetCommitted);
      expect(sessions.isAuthenticated, isFalse);
      expect(store.current!.session, isNull);
      await flows.acknowledgeResetResult();
      await sessions.login(username, password);
      expect(sessions.isAuthenticated, isFalse);
      await sessions.login(username, resetPassword);
      expect(sessions.isAuthenticated, isTrue);

      final closingToken = store.current!.session!.token;
      final closingAccount = store.current!.session!.accountId;
      final closingIdentities = await api.identities(closingToken);
      expect(
        (await api.identityChangeResult(
          sessionToken: closingToken,
          idempotencyKey: identityReceiptKey,
        )).committed,
        isTrue,
      );
      await fixture('capture-identities', {'token': closingToken});
      await fixture('drop-next', {'path': '/api/v1/account-closures'});
      await flows.requestClosure(resetPassword);
      expect(flows.status, AuthFlowStatus.closureUnknown);
      final closureId = store.current!.pendingClosure!['closureId'];
      final closureSecret = store.current!.pendingClosure!['statusSecret'];
      expect(sessions.isAuthenticated, isFalse);
      await restartClient();
      expect(flows.status, AuthFlowStatus.closureUnknown);
      await flows.reconcileClosure();
      expect(flows.status, AuthFlowStatus.closurePending);
      expect(store.current!.pendingClosure!['originalBearer'], isNull);
      expect(store.current!.pendingClosure!['closureId'], closureId);
      expect(store.current!.pendingClosure!['statusSecret'], closureSecret);
      await fixture('identities-unchanged', {'state': 'PENDING_CLOSE'});
      await sessions.login(username, resetPassword);
      expect(sessions.isAuthenticated, isTrue);
      await flows.reconcileClosure();
      expect(flows.status, AuthFlowStatus.closureCancelled);
      await fixture('identities-unchanged', {'state': 'ACTIVE'});
      await flows.forgetClosureStatus();
      await flows.requestClosure(resetPassword);
      expect(flows.status, AuthFlowStatus.closurePending);
      final finalClosureId =
          store.current!.pendingClosure!['closureId'] as String;
      await fixture('identities-unchanged', {'state': 'PENDING_CLOSE'});
      await fixture('finalize');
      await fixture('identities-finalized', {'closureId': finalClosureId});
      await fixture('finalize-again');
      await fixture('identities-finalized', {'closureId': finalClosureId});
      await expectLater(
        api.identities(closingToken),
        throwsA(
          isA<AuthFailure>().having(
            (e) => e.unauthorized,
            'closed bearer',
            true,
          ),
        ),
      );
      await expectLater(
        api.identityChangeResult(
          sessionToken: closingToken,
          idempotencyKey: identityReceiptKey,
        ),
        throwsA(
          isA<AuthFailure>().having(
            (e) => e.unauthorized,
            'closed receipt access',
            true,
          ),
        ),
      );
      await flows.reconcileClosure();
      expect(flows.status, AuthFlowStatus.closureClosedReleasePending);
      await fixture('deliver-release', {'slotId': slot});
      await flows.reconcileClosure();
      expect(flows.status, AuthFlowStatus.closureReleased);
      expect(flows.closureStatus!.releaseReceipt, isNotEmpty);
      expect(sessions.isAuthenticated, isFalse);
      await sessions.login(username, resetPassword);
      expect(sessions.isAuthenticated, isFalse);

      // Register the same exact V address with new bootstrap material after
      // release. C's finalizer advanced seven days; align V's signed fixture
      // clock, without changing any SQL deadline or resurrecting an old flow.
      await flows.forgetClosureStatus();
      await fixture('advance-verifier-after-closure');
      clientClock = clientClock.add(const Duration(days: 7, seconds: 1));
      await flows.requestOtp(email);
      expect(flows.status, AuthFlowStatus.otpRequested);
      final returnMail = await fixture('otp', {'email': email});
      await flows.confirmOtp(returnMail['code'] as String);
      expect(flows.status, AuthFlowStatus.eligibilityReady);
      await flows.createRegistration('synthetic_return', password);
      expect(flows.status, AuthFlowStatus.registrationCodeShown);
      final returnCode = flows.recoveryCode!;
      flows.hideRecoveryCode();
      await flows.commitRegistration(returnCode);
      expect(flows.status, AuthFlowStatus.registrationComplete);
      expect(sessions.isAuthenticated, isFalse);
      await sessions.login('synthetic_return', password);
      expect(sessions.isAuthenticated, isTrue);
      final returned = store.current!.session!;
      expect(returned.accountId, isNot(closingAccount));
      final empty = await api.identities(returned.token);
      expect(empty.identities, isEmpty);
      expect(empty.createdCount, 0);
      expect(empty.nextCreateAt, isNull);
      final oldReceipt = await api.identityChangeResult(
        sessionToken: returned.token,
        idempotencyKey: identityReceiptKey,
      );
      expect(oldReceipt.committed, isFalse);
      expect(oldReceipt.rejected, isFalse);
      expect(oldReceipt.identityId, isNull);
      final first = await api.changeIdentity(
        sessionToken: returned.token,
        idempotencyKey: AuthCrypto.randomEncoded(16),
        operation: IdentityOperation.create,
        nickname: '海风',
      );
      expect(first.committed, isTrue);
      expect(
        closingIdentities.identities.map((i) => i.id),
        isNot(contains(first.identityId)),
      );
      final independent = await api.identities(returned.token);
      expect(independent.createdCount, 1);
      expect(independent.identities.single.isOriginal, isTrue);
      expect(independent.nextCreateAt, isNull);
      await fixture('new-account-identities', {
        'accountId': returned.accountId,
        'closureId': finalClosureId,
      });
    },
    timeout: const Timeout(Duration(minutes: 4)),
  );
}
