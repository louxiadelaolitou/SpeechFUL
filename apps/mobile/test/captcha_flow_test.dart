import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/src/auth/auth_flows.dart';
import 'package:hnuhole_mobile/src/auth/auth_models.dart';
import 'package:hnuhole_mobile/src/auth/auth_store.dart';
import 'package:hnuhole_mobile/src/auth/captcha.dart';

import 'auth_flows_test.dart' as f;
import 'email_reset_test.dart' as reset;

class _Api extends reset.ResetApi {
  bool cancel = false;
  @override
  Future<OtpRequestAccepted> requestEmailLoginOtp({
    required String email,
    required String installationId,
    required String idempotencyKey,
  }) async {
    if (cancel) throw const CaptchaNotSubmitted(cancelled: true);
    return super.requestEmailLoginOtp(
      email: email,
      installationId: installationId,
      idempotencyKey: idempotencyKey,
    );
  }
}

void main() {
  for (final reset in [false, true]) {
    for (final previous in ['none', 'accepted', 'unknown']) {
      test(
        'captcha cancellation preserves $previous mail state; reset=$reset',
        () async {
          final api = _Api(), store = AuthStore(f.Vault());
          var clock = f.now;
          final flow = AuthFlows(
            api: api,
            store: store,
            clock: () => clock,
            acceptRegistrationSession: (_) async {},
            clearCommunityAccess: () {},
          );
          Future<void> request() => reset
              ? flow.requestEmailReset('owned@hainanu.edu.cn')
              : flow.requestEmailLogin('owned@hainanu.edu.cn');
          if (previous != 'none') {
            api.sendTimeout = previous == 'unknown';
            await request();
            clock = clock.add(const Duration(seconds: 61));
          }
          final before = reset
              ? store.current?.emailReset
              : store.current?.emailLogin;
          api.cancel = true;
          api.sendTimeout = false;
          await request();
          expect(
            reset ? store.current?.emailReset : store.current?.emailLogin,
            before,
          );
          expect(flow.errorMessage, isNull);
          expect(flow.busy, isFalse);
          if (previous == 'none') {
            api.cancel = false;
            await request();
            expect(
              (reset
                  ? store.current?.emailReset
                  : store.current?.emailLogin)?['sendState'],
              'ACCEPTED',
            );
          }
          flow.dispose();
          store.dispose();
        },
      );
    }
  }
  for (final unknown in [false, true]) {
    test(
      'registration cancel preserves bootstrap and unknown=$unknown',
      () async {
        final api = f.Api(), store = AuthStore(f.Vault());
        final flow = AuthFlows(
          api: api,
          store: store,
          clock: () => f.now,
          acceptRegistrationSession: (_) async {},
          clearCommunityAccess: () {},
        );
        if (unknown) {
          api.onOtp = () async =>
              throw const AuthFailure(kind: AuthFailureKind.timeout);
          await flow.requestOtp('owned@hainanu.edu.cn');
        }
        final before = store.current?.registration?['request'];
        api.onOtp = () async =>
            throw const CaptchaNotSubmitted(cancelled: true);
        await flow.requestOtp('owned@hainanu.edu.cn');
        expect(store.current?.registration?['request'], before);
        expect(store.current?.registration?['seed'], isNotNull);
        expect(flow.errorMessage, isNull);
        expect(
          flow.status,
          unknown ? AuthFlowStatus.otpRequestUnknown : AuthFlowStatus.idle,
        );
        flow.dispose();
        store.dispose();
      },
    );
  }
}
