import 'dart:convert';

import '../auth/auth_crypto.dart';
import '../auth/auth_store.dart';

/// Each account uses its own native, non-backed-up installation namespace.
class StudentLetterStore {
  StudentLetterStore({required this.vaultFor});

  factory StudentLetterStore.native({
    required String environment,
  }) => StudentLetterStore(
    vaultFor: (accountId) => FlutterAuthVault(
      namespace:
          'hnuhole.student-letter.v1.${AuthCrypto.domainDigest('HNUHOLE/STUDENT-LETTER/V1', utf8.encode(jsonEncode([environment, accountId])))}',
    ),
  );

  final AuthVault Function(String accountId) vaultFor;
  static const _receipt = 'student-letter-v1:read';

  Future<bool> hasRead(String accountId) async =>
      await vaultFor(accountId).read() == _receipt;

  Future<void> markRead(String accountId) async {
    final vault = vaultFor(accountId);
    await vault.write(_receipt);
    if (await vault.read() != _receipt) throw const AuthStorageFailure();
  }
}
